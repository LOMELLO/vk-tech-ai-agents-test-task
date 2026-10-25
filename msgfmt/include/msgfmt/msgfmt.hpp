#pragma once

#include <cstddef>
#include <cstdint>
#include <string>
#include <string_view>
#include <vector>

namespace msgfmt {

struct Range {
    std::string style;
    std::uint32_t offset = 0;
    std::uint32_t length = 0;
    std::string url;
};

struct ParseResult {
    std::string text;
    std::vector<Range> ranges;
};

bool parse(std::string_view input, ParseResult& result);

bool escape(std::string_view input, std::string& output);

std::string to_json(const ParseResult& result);

std::string error_json();

}
