cmake_minimum_required(VERSION 3.20)
project(msgfmt LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

option(MSGFMT_SANITIZE "Build with AddressSanitizer and UndefinedBehaviorSanitizer" OFF)
option(MSGFMT_BUILD_TESTS "Build msgfmt tests" ON)

add_library(msgfmt_lib STATIC src/msgfmt.cpp)
target_include_directories(msgfmt_lib PUBLIC ${CMAKE_CURRENT_SOURCE_DIR}/include)
target_compile_options(msgfmt_lib PRIVATE -Wall -Wextra)

add_executable(msgfmt src/main.cpp)
target_link_libraries(msgfmt PRIVATE msgfmt_lib)
target_compile_options(msgfmt PRIVATE -Wall -Wextra)

if(MSGFMT_SANITIZE)
    target_compile_options(msgfmt_lib PUBLIC -fsanitize=address,undefined -fno-omit-frame-pointer)
    target_link_options(msgfmt_lib PUBLIC -fsanitize=address,undefined)
endif()

if(MSGFMT_BUILD_TESTS)
    enable_testing()
    add_executable(msgfmt_tests tests/test_msgfmt.cpp)
    target_link_libraries(msgfmt_tests PRIVATE msgfmt_lib)
    target_compile_options(msgfmt_tests PRIVATE -Wall -Wextra)
    add_test(NAME msgfmt_tests COMMAND msgfmt_tests)
endif()
