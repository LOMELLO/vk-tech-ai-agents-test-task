#include "msgfmt/msgfmt.hpp"

#include <iostream>
#include <iterator>
#include <string>
#include <string_view>

int main(int argc, char** argv) {
    bool escape_mode = false;
    if (argc == 2) {
        if (std::string_view(argv[1]) != "escape") {
            std::cerr << "msgfmt: unknown command\n";
            return 2;
        }
        escape_mode = true;
    } else if (argc > 2) {
        std::cerr << "msgfmt: too many arguments\n";
        return 2;
    }

    const std::string input{std::istreambuf_iterator<char>(std::cin),
                            std::istreambuf_iterator<char>()};

    if (escape_mode) {
        std::string output;
        if (!msgfmt::escape(input, output)) {
            std::cout << msgfmt::error_json();
            return 2;
        }
        std::cout.write(output.data(), static_cast<std::streamsize>(output.size()));
        return 0;
    }

    msgfmt::ParseResult result;
    if (!msgfmt::parse(input, result)) {
        std::cout << msgfmt::error_json();
        return 2;
    }
    std::cout << msgfmt::to_json(result);
    return 0;
}
