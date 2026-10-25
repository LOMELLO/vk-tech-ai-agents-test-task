#include "msgfmt/msgfmt.hpp"

#include <algorithm>
#include <array>
#include <cstdint>
#include <optional>
#include <string>
#include <utility>
#include <vector>

namespace msgfmt {
namespace {

bool is_service(char32_t c) {
    switch (c) {
        case U'*':
        case U'_':
        case U'`':
        case U'[':
        case U']':
        case U'(':
        case U')':
        case U'\\':
            return true;
        default:
            return false;
    }
}

bool is_space(char32_t c) {
    if (c == 0x09 || c == 0x0A || c == 0x0B || c == 0x0C || c == 0x0D || c == 0x20) {
        return true;
    }
    if (c == 0x85 || c == 0xA0 || c == 0x1680) {
        return true;
    }
    if (c >= 0x2000 && c <= 0x200A) {
        return true;
    }
    if (c == 0x2028 || c == 0x2029 || c == 0x202F || c == 0x205F || c == 0x3000) {
        return true;
    }
    return false;
}

bool is_word_codepoint(char32_t c) {
    if ((c >= U'0' && c <= U'9') || (c >= U'A' && c <= U'Z') || (c >= U'a' && c <= U'z')) {
        return true;
    }
    static constexpr std::array<std::pair<char32_t, char32_t>, 66> ranges{{
        {0x00AA, 0x00AA}, {0x00B5, 0x00B5}, {0x00BA, 0x00BA}, {0x00C0, 0x02FF},
        {0x0300, 0x036F}, {0x0370, 0x052F}, {0x0531, 0x058F}, {0x0590, 0x05FF},
        {0x0600, 0x06FF}, {0x0700, 0x077F}, {0x0780, 0x07FF}, {0x0900, 0x0DFF},
        {0x0E00, 0x0EFF}, {0x1000, 0x109F}, {0x10A0, 0x10FF}, {0x1100, 0x11FF},
        {0x1200, 0x139F}, {0x13A0, 0x13FF}, {0x1400, 0x167F}, {0x1680, 0x169F},
        {0x16A0, 0x16FF}, {0x1700, 0x18AF}, {0x1C80, 0x1DBF}, {0x1E00, 0x1FFF},
        {0x2070, 0x209F}, {0x20D0, 0x20FF}, {0x2100, 0x214F}, {0x2C00, 0x2DFF},
        {0x2E80, 0x2FFF}, {0x3040, 0x30FF}, {0x3100, 0x31BF}, {0x31F0, 0x31FF},
        {0x3400, 0x4DBF}, {0x4E00, 0x9FFF}, {0xA000, 0xA4CF}, {0xA500, 0xA63F},
        {0xA640, 0xA7FF}, {0xA800, 0xA97F}, {0xA980, 0xAA5F}, {0xAC00, 0xD7AF},
        {0xF900, 0xFAFF}, {0xFB00, 0xFB4F}, {0xFB50, 0xFDFF}, {0xFE20, 0xFE2F},
        {0xFE70, 0xFEFF}, {0xFF10, 0xFF19}, {0xFF21, 0xFF3A}, {0xFF41, 0xFF5A},
        {0xFF66, 0xFF9D}, {0x10000, 0x1007F}, {0x10080, 0x100FF}, {0x10140, 0x1018F},
        {0x10280, 0x1029F}, {0x10300, 0x1034F}, {0x10400, 0x1044F}, {0x1D400, 0x1D7FF},
        {0x20000, 0x2A6DF}, {0x2A700, 0x2B73F}, {0x2B740, 0x2B81F}, {0x2B820, 0x2CEAF},
        {0x2CEB0, 0x2EBEF}, {0x30000, 0x3134F}, {0x1E900, 0x1E95F}, {0x1EE00, 0x1EEFF},
        {0x1F100, 0x1F12F}, {0x1F130, 0x1F149},
    }};
    for (const auto& range : ranges) {
        if (c >= range.first && c <= range.second) {
            return true;
        }
    }
    return false;
}

std::uint32_t utf16_width(char32_t c) {
    return c > 0xFFFF ? 2U : 1U;
}

bool decode_utf8(std::string_view input, std::vector<char32_t>& output) {
    std::size_t i = 0;
    const std::size_t n = input.size();
    while (i < n) {
        const unsigned char b0 = static_cast<unsigned char>(input[i]);
        char32_t cp = 0;
        std::size_t length = 0;
        if (b0 <= 0x7F) {
            cp = b0;
            length = 1;
        } else if (b0 >= 0xC2 && b0 <= 0xDF) {
            cp = b0 & 0x1F;
            length = 2;
        } else if (b0 >= 0xE0 && b0 <= 0xEF) {
            cp = b0 & 0x0F;
            length = 3;
        } else if (b0 >= 0xF0 && b0 <= 0xF4) {
            cp = b0 & 0x07;
            length = 4;
        } else {
            return false;
        }
        if (i + length > n) {
            return false;
        }
        for (std::size_t j = 1; j < length; ++j) {
            const unsigned char b = static_cast<unsigned char>(input[i + j]);
            if ((b & 0xC0) != 0x80) {
                return false;
            }
            cp = (cp << 6) | (b & 0x3F);
        }
        if (length == 2 && cp < 0x80) {
            return false;
        }
        if (length == 3 && cp < 0x800) {
            return false;
        }
        if (length == 4 && cp < 0x10000) {
            return false;
        }
        if (cp > 0x10FFFF || (cp >= 0xD800 && cp <= 0xDFFF)) {
            return false;
        }
        output.push_back(cp);
        i += length;
    }
    return true;
}

void append_utf8(std::string& output, char32_t c) {
    if (c <= 0x7F) {
        output.push_back(static_cast<char>(c));
    } else if (c <= 0x7FF) {
        output.push_back(static_cast<char>(0xC0 | (c >> 6)));
        output.push_back(static_cast<char>(0x80 | (c & 0x3F)));
    } else if (c <= 0xFFFF) {
        output.push_back(static_cast<char>(0xE0 | (c >> 12)));
        output.push_back(static_cast<char>(0x80 | ((c >> 6) & 0x3F)));
        output.push_back(static_cast<char>(0x80 | (c & 0x3F)));
    } else {
        output.push_back(static_cast<char>(0xF0 | (c >> 18)));
        output.push_back(static_cast<char>(0x80 | ((c >> 12) & 0x3F)));
        output.push_back(static_cast<char>(0x80 | ((c >> 6) & 0x3F)));
        output.push_back(static_cast<char>(0x80 | (c & 0x3F)));
    }
}

std::string encode_utf8(const std::vector<char32_t>& input) {
    std::string output;
    for (char32_t c : input) {
        append_utf8(output, c);
    }
    return output;
}

struct Item {
    Range range;
    std::uint32_t sequence;
};

class Parser {
public:
    explicit Parser(const std::vector<char32_t>& input) : input_(input) {}

    ParseResult run() {
        parse_content(input_.size(), std::nullopt);
        std::sort(items_.begin(), items_.end(), [](const Item& a, const Item& b) {
            if (a.range.offset != b.range.offset) {
                return a.range.offset < b.range.offset;
            }
            if (a.range.length != b.range.length) {
                return a.range.length > b.range.length;
            }
            if (a.sequence != b.sequence) {
                return a.sequence < b.sequence;
            }
            if (a.range.style != b.range.style) {
                return a.range.style < b.range.style;
            }
            return a.range.url < b.range.url;
        });
        ParseResult result;
        result.text = encode_utf8(output_);
        result.ranges.reserve(items_.size());
        for (const auto& item : items_) {
            result.ranges.push_back(item.range);
        }
        return result;
    }

private:
    enum class Close {
        None,
        Bold,
        Italic,
    };

    struct Snapshot {
        std::size_t position;
        std::size_t output_size;
        std::size_t items_size;
        std::uint32_t utf16;
        std::uint32_t sequence;
    };

    Snapshot snapshot() const {
        return {pos_, output_.size(), items_.size(), utf16_, sequence_};
    }

    void restore(const Snapshot& s) {
        pos_ = s.position;
        output_.resize(s.output_size);
        items_.resize(s.items_size);
        utf16_ = s.utf16;
        sequence_ = s.sequence;
    }

    void emit(char32_t c) {
        output_.push_back(c);
        utf16_ += utf16_width(c);
    }

    void add_range(const std::string& style, std::uint32_t offset, std::uint32_t length,
                   std::string url, std::uint32_t sequence) {
        items_.push_back(Item{Range{style, offset, length, std::move(url)}, sequence});
    }

    bool parse_content(std::size_t limit, std::optional<Close> close) {
        while (pos_ < limit) {
            const char32_t c = input_[pos_];
            if (c == U'\\' && pos_ + 1 < limit && is_service(input_[pos_ + 1])) {
                emit(input_[pos_ + 1]);
                pos_ += 2;
                continue;
            }
            if (close == Close::Bold && c == U'*' && pos_ + 1 < limit && input_[pos_ + 1] == U'*') {
                if (pos_ > 0 && !is_space(input_[pos_ - 1])) {
                    pos_ += 2;
                    return true;
                }
                emit(U'*');
                emit(U'*');
                pos_ += 2;
                continue;
            }
            if (close == Close::Italic && c == U'_') {
                const bool previous_non_space = pos_ > 0 && !is_space(input_[pos_ - 1]);
                const bool next_boundary = pos_ + 1 >= limit || !is_word_codepoint(input_[pos_ + 1]);
                if (previous_non_space && next_boundary) {
                    ++pos_;
                    return true;
                }
                emit(U'_');
                ++pos_;
                continue;
            }
            if (c == U'*' && pos_ + 1 < limit && input_[pos_ + 1] == U'*') {
                const bool next_non_space = pos_ + 2 < limit && !is_space(input_[pos_ + 2]);
                if (next_non_space && try_style(Close::Bold, 2, limit)) {
                    continue;
                }
                emit(U'*');
                emit(U'*');
                pos_ += 2;
                continue;
            }
            if (c == U'_') {
                const bool before_boundary = pos_ == 0 || !is_word_codepoint(input_[pos_ - 1]);
                const bool next_non_space = pos_ + 1 < limit && !is_space(input_[pos_ + 1]);
                if (before_boundary && next_non_space && try_style(Close::Italic, 1, limit)) {
                    continue;
                }
                emit(U'_');
                ++pos_;
                continue;
            }
            if (c == U'`') {
                if (pos_ + 1 < limit && !is_space(input_[pos_ + 1]) && try_code(limit)) {
                    continue;
                }
                emit(U'`');
                ++pos_;
                continue;
            }
            if (c == U'[') {
                if (try_link(limit)) {
                    continue;
                }
                emit(U'[');
                ++pos_;
                continue;
            }
            emit(c);
            ++pos_;
        }
        return false;
    }

    bool try_style(Close close, std::size_t marker_length, std::size_t limit) {
        const Snapshot saved = snapshot();
        const std::size_t content_start = pos_ + marker_length;
        const std::uint32_t start_offset = utf16_;
        const std::uint32_t sequence = sequence_++;
        pos_ = content_start;
        const bool closed = parse_content(limit, close);
        if (!closed) {
            restore(saved);
            return false;
        }
        const std::string style = close == Close::Bold ? "bold" : "italic";
        add_range(style, start_offset, utf16_ - start_offset, "", sequence);
        return true;
    }

    bool try_code(std::size_t limit) {
        const Snapshot saved = snapshot();
        const std::size_t content_start = pos_ + 1;
        const std::uint32_t start_offset = utf16_;
        const std::uint32_t sequence = sequence_++;
        std::size_t i = content_start;
        while (i < limit) {
            if (input_[i] == U'`' && i > 0 && !is_space(input_[i - 1])) {
                for (std::size_t j = content_start; j < i; ++j) {
                    emit(input_[j]);
                }
                pos_ = i + 1;
                add_range("code", start_offset, utf16_ - start_offset, "", sequence);
                return true;
            }
            ++i;
        }
        restore(saved);
        return false;
    }

    enum class LinkScan {
        NotLink,
        Invalid,
        Valid,
    };

    LinkScan scan_link(std::size_t limit, std::size_t& text_end, std::size_t& url_start,
                       std::size_t& url_end, std::size_t& after) const {
        std::size_t i = pos_ + 1;
        bool found_text_end = false;
        while (i < limit) {
            if (input_[i] == U'\\' && i + 1 < limit && is_service(input_[i + 1])) {
                i += 2;
                continue;
            }
            if (input_[i] == U']') {
                found_text_end = true;
                break;
            }
            ++i;
        }
        if (!found_text_end) {
            return LinkScan::NotLink;
        }
        text_end = i;
        if (is_space(input_[pos_ + 1])) {
            return LinkScan::NotLink;
        }
        if (text_end + 1 >= limit || input_[text_end + 1] != U'(') {
            return LinkScan::NotLink;
        }
        std::size_t j = text_end + 2;
        while (j < limit && input_[j] != U')') {
            ++j;
        }
        if (j >= limit) {
            return LinkScan::NotLink;
        }
        url_start = text_end + 2;
        url_end = j;
        after = j + 1;
        if (text_end == pos_ + 1 || url_start == url_end) {
            return LinkScan::Invalid;
        }
        for (std::size_t k = url_start; k < url_end; ++k) {
            if (is_space(input_[k]) || input_[k] == U')') {
                return LinkScan::Invalid;
            }
        }
        return LinkScan::Valid;
    }

    void emit_literal_span(std::size_t begin, std::size_t end) {
        std::size_t i = begin;
        while (i < end) {
            if (input_[i] == U'\\' && i + 1 < end && is_service(input_[i + 1])) {
                emit(input_[i + 1]);
                i += 2;
            } else {
                emit(input_[i]);
                ++i;
            }
        }
    }

    bool try_link(std::size_t limit) {
        std::size_t text_end = 0;
        std::size_t url_start = 0;
        std::size_t url_end = 0;
        std::size_t after = 0;
        const LinkScan scan = scan_link(limit, text_end, url_start, url_end, after);
        if (scan == LinkScan::NotLink) {
            return false;
        }
        if (scan == LinkScan::Invalid) {
            emit_literal_span(pos_, after);
            pos_ = after;
            return true;
        }
        std::string url;
        for (std::size_t i = url_start; i < url_end; ++i) {
            append_utf8(url, input_[i]);
        }
        const std::uint32_t start_offset = utf16_;
        const std::uint32_t sequence = sequence_++;
        pos_ = pos_ + 1;
        parse_content(text_end, std::nullopt);
        pos_ = after;
        add_range("link", start_offset, utf16_ - start_offset, std::move(url), sequence);
        return true;
    }

    const std::vector<char32_t>& input_;
    std::size_t pos_ = 0;
    std::vector<char32_t> output_;
    std::uint32_t utf16_ = 0;
    std::uint32_t sequence_ = 0;
    std::vector<Item> items_;
};

std::string json_string(std::string_view value) {
    std::string result;
    result.push_back('"');
    for (unsigned char c : value) {
        switch (c) {
            case '"':
                result += "\\\"";
                break;
            case '\\':
                result += "\\\\";
                break;
            case '\b':
                result += "\\b";
                break;
            case '\f':
                result += "\\f";
                break;
            case '\n':
                result += "\\n";
                break;
            case '\r':
                result += "\\r";
                break;
            case '\t':
                result += "\\t";
                break;
            default:
                if (c < 0x20) {
                    static constexpr char hex[] = "0123456789abcdef";
                    result += "\\u00";
                    result.push_back(hex[(c >> 4) & 0x0F]);
                    result.push_back(hex[c & 0x0F]);
                } else {
                    result.push_back(static_cast<char>(c));
                }
                break;
        }
    }
    result.push_back('"');
    return result;
}

}

bool parse(std::string_view input, ParseResult& result) {
    std::vector<char32_t> codepoints;
    if (!decode_utf8(input, codepoints)) {
        return false;
    }
    Parser parser(codepoints);
    result = parser.run();
    return true;
}

bool escape(std::string_view input, std::string& output) {
    std::vector<char32_t> codepoints;
    if (!decode_utf8(input, codepoints)) {
        return false;
    }
    output.clear();
    for (char32_t c : codepoints) {
        if (is_service(c)) {
            output.push_back('\\');
        }
        append_utf8(output, c);
    }
    return true;
}

std::string to_json(const ParseResult& result) {
    std::string output = "{\"text\": ";
    output += json_string(result.text);
    output += ", \"ranges\": [";
    for (std::size_t i = 0; i < result.ranges.size(); ++i) {
        if (i != 0) {
            output += ", ";
        }
        const Range& range = result.ranges[i];
        output += "{\"style\": ";
        output += json_string(range.style);
        output += ", \"offset\": ";
        output += std::to_string(range.offset);
        output += ", \"length\": ";
        output += std::to_string(range.length);
        output += ", \"url\": ";
        output += json_string(range.url);
        output += "}";
    }
    output += "]}";
    return output;
}

std::string error_json() {
    return "{\"error\": \"invalid_utf8\"}";
}

}
