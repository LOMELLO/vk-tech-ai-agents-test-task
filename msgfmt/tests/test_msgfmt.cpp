#include "msgfmt/msgfmt.hpp"

#include <cstdint>
#include <cstdlib>
#include <iostream>
#include <string>
#include <string_view>
#include <vector>

namespace {

int failures = 0;

void fail(const char* expression, const char* file, int line) {
    std::cerr << file << ':' << line << ": check failed: " << expression << '\n';
    ++failures;
}

#define CHECK(expression)             \
    do {                              \
        if (!(expression)) {          \
            fail(#expression, __FILE__, __LINE__); \
        }                             \
    } while (false)

bool parse_ok(std::string_view input, msgfmt::ParseResult& result) {
    return msgfmt::parse(input, result);
}

void expect_ranges(const msgfmt::ParseResult& result,
                   const std::vector<msgfmt::Range>& expected) {
    CHECK(result.ranges.size() == expected.size());
    if (result.ranges.size() != expected.size()) {
        return;
    }
    for (std::size_t i = 0; i < expected.size(); ++i) {
        if (result.ranges[i].style != expected[i].style ||
            result.ranges[i].offset != expected[i].offset ||
            result.ranges[i].length != expected[i].length ||
            result.ranges[i].url != expected[i].url) {
            std::cerr << "range " << i << ": actual " << result.ranges[i].style << '('
                      << result.ranges[i].offset << ',' << result.ranges[i].length << ','
                      << result.ranges[i].url << ") expected " << expected[i].style << '('
                      << expected[i].offset << ',' << expected[i].length << ','
                      << expected[i].url << ")\n";
        }
        CHECK(result.ranges[i].style == expected[i].style);
        CHECK(result.ranges[i].offset == expected[i].offset);
        CHECK(result.ranges[i].length == expected[i].length);
        CHECK(result.ranges[i].url == expected[i].url);
    }
}

void case_parse(std::string_view input, std::string_view text,
                const std::vector<msgfmt::Range>& ranges) {
    msgfmt::ParseResult result;
    CHECK(parse_ok(input, result));
    if (result.text != text) {
        std::cerr << "input: " << input << "\nactual text: " << result.text
                  << "\nexpected text: " << text << '\n';
    }
    CHECK(result.text == text);
    if (result.ranges.size() != ranges.size()) {
        std::cerr << "input: " << input << "\nranges: " << result.ranges.size()
                  << " expected: " << ranges.size() << '\n';
    }
    expect_ranges(result, ranges);
}

void test_examples() {
    case_parse("**важно**", "важно", {{"bold", 0, 5, ""}});
    case_parse("Привет, **мир**!", "Привет, мир!", {{"bold", 8, 3, ""}});
    case_parse("_курсив_", "курсив", {{"italic", 0, 6, ""}});
    case_parse("a_b_c", "a_b_c", {});
    case_parse("**не закрыто", "**не закрыто", {});
    case_parse("2 \\* 3", "2 * 3", {});
    case_parse("`чат * список`", "чат * список", {{"code", 0, 12, ""}});
    case_parse("[документация](https://github.com)", "документация",
               {{"link", 0, 12, "https://github.com"}});
    case_parse("**жирный _и_ курсив**", "жирный и курсив",
               {{"bold", 0, 15, ""}, {"italic", 7, 1, ""}});
    case_parse("🚀 **ок**", "🚀 ок", {{"bold", 3, 2, ""}});
    case_parse("** 🚀 ок**", "** 🚀 ок**", {});
    case_parse("[текст]()", "[текст]()", {});
    case_parse("\\\\", "\\", {});
    case_parse("", "", {});
    case_parse("_ e_", "_ e_", {});
}

void test_parsing_details() {
    case_parse("**_a_**", "a", {{"bold", 0, 1, ""}, {"italic", 0, 1, ""}});
    case_parse("**[a](u)**", "a", {{"bold", 0, 1, ""}, {"link", 0, 1, "u"}});
    case_parse("`**a**`", "**a**", {{"code", 0, 5, ""}});
    case_parse("**a `b**c` d**", "a b**c d", {{"bold", 0, 8, ""}, {"code", 2, 4, ""}});
    case_parse("_a_b_", "a_b", {{"italic", 0, 3, ""}});
    case_parse("x _a_ y", "x a y", {{"italic", 2, 1, ""}});
    case_parse("Привет_мир_", "Привет_мир_", {});
    case_parse("\\_a_", "_a_", {});
    case_parse("[a](u v)", "[a](u v)", {});
    case_parse("[a](u)", "a", {{"link", 0, 1, "u"}});
    case_parse("[a](u)(b)", "a(b)", {{"link", 0, 1, "u"}});
    case_parse("[a\\]b](u)", "a]b", {{"link", 0, 3, "u"}});
    case_parse("**a****", "a**", {{"bold", 0, 1, ""}});
}

void test_invalid_utf8() {
    msgfmt::ParseResult result;
    CHECK(!msgfmt::parse(std::string("\xC0\xAF", 2), result));
    CHECK(!msgfmt::parse(std::string("\xED\xA0\x80", 3), result));
    CHECK(!msgfmt::parse(std::string("\xF4\x90\x80\x80", 4), result));
    CHECK(!msgfmt::parse(std::string("\xE2\x82", 2), result));
}

void test_escape() {
    std::string output;
    CHECK(msgfmt::escape("2 * 3", output));
    CHECK(output == "2 \\* 3");
    CHECK(msgfmt::escape("a_b`c[d](e)\\f", output));
    CHECK(output == "a\\_b\\`c\\[d\\]\\(e\\)\\\\f");
    CHECK(msgfmt::escape("🚀", output));
    CHECK(output == "🚀");
    CHECK(!msgfmt::escape(std::string("\xFF", 1), output));
}

void test_json() {
    msgfmt::ParseResult result;
    CHECK(parse_ok("**a\"b**", result));
    const std::string json = msgfmt::to_json(result);
    CHECK(json == "{\"text\": \"a\\\"b\", \"ranges\": [{\"style\": \"bold\", \"offset\": 0, "
                  "\"length\": 3, \"url\": \"\"}]}");
    CHECK(msgfmt::error_json() == "{\"error\": \"invalid_utf8\"}");
}

}

int main() {
    test_examples();
    test_parsing_details();
    test_invalid_utf8();
    test_escape();
    test_json();
    if (failures != 0) {
        std::cerr << failures << " checks failed\n";
        return EXIT_FAILURE;
    }
    std::cout << "all checks passed\n";
    return EXIT_SUCCESS;
}
