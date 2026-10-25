# Тестовое задание VK Tech

Три части: библиотека `msgfmt` на C++17, служба `ghreport` на Go и интеграционные тесты на Python.

## Структура

```text
msgfmt/      C++17 библиотека и CLI
service/     Go-служба, Dockerfile, unit-тесты
api_tests/   pytest, встроенный mock-сервер GitHub API
```

## Сборка и проверки

```sh
cmake -S msgfmt -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build
ctest --test-dir build

cd service
go test ./...
go vet ./...
gofmt -l .
go build -o ghreport .
cd ..

GHREPORT_BIN="$PWD/service/ghreport" python3 -m pytest api_tests -q
```

Docker:

```sh
docker build -t ghreport service/
docker run --rm -e GITHUB_TOKEN=... ghreport --repos cli/cli --limit 20 --out -
```

## Решения

- `msgfmt` использует только стандартную библиотеку, разбирает UTF-8 вручную и считает позиции в UTF-16.
- `ghreport` использует стандартный `net/http`, сам разбирает `Link`, JSON и заголовки лимитов; сторонние SDK не применяются.
- Кэш хранит ETag, тело и `Link`; токен в кэш и журнал не попадает.
- Интеграционные тесты поднимают локальный сервер, соответствующий контракту, поэтому не зависят от внешней сети.

## Ограничения

- Граница слова для `_` определяется таблицей распространённых диапазонов Unicode, а не полной базой Unicode.
- В контейнере по умолчанию используется `https://api.github.com`; другой адрес задаётся через `GITHUB_API_BASE_URL`.

## Метаданные

Затраченное время: около 6 часов.

Использованные ИИ-инструменты: claude.
