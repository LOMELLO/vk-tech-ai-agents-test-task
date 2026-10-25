# msgfmt

Библиотека и CLI для преобразования подмножества Markdown в чистый текст и диапазоны стилей в кодовых единицах UTF-16.

## Сборка

```sh
cmake -S msgfmt -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build
ctest --test-dir build
```

Санитайзеры:

```sh
cmake -S msgfmt -B build-san -DCMAKE_BUILD_TYPE=Debug -DMSGFMT_SANITIZE=ON
cmake --build build-san
ctest --test-dir build-san
```

## Запуск

```sh
printf '2 * 3' | build/msgfmt escape
printf 'Привет, **мир**!' | build/msgfmt
```

`msgfmt` читает UTF-8 из stdin и пишет JSON в stdout. `msgfmt escape` экранирует служебные символы. Для некорректного UTF-8 выводится `{"error": "invalid_utf8"}` и код возврата 2.

## Решения

- Однопроходный разбор с рекурсивной обработкой вложенных стилей.
- Неудачная конструкция откатывается и выводится литералами.
- Позиции считаются после удаления разметки; символы вне BMP занимают две единицы UTF-16.
- Используется только стандартная библиотека C++17.

## Ограничения

- Классификация букв и цифр для границы слова реализована таблицей распространённых диапазонов Unicode, а не полной базой Unicode.
- Внутри `code` разметка и экранирование не интерпретируются.
