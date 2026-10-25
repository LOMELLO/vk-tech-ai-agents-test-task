# ghreport

Служба на Go формирует JSON-сводку по открытым issue репозиториев GitHub.

## Запуск

```sh
go build -o ghreport .
./ghreport --repos cli/cli --limit 20 --out -
```

Docker:

```sh
docker build -t ghreport service/
docker run --rm -e GITHUB_TOKEN=... ghreport --repos cli/cli --limit 20 --out -
```

Контейнер работает от пользователя `ghreport` (UID 10001). Токен и адрес API передаются только через окружение.

## Переменные окружения

| Переменная | Назначение |
| --- | --- |
| `GITHUB_TOKEN` | Необязательный Bearer-токен |
| `GITHUB_API_BASE_URL` | Базовый адрес API, по умолчанию `https://api.github.com` |

## Проверки

```sh
go test ./...
go vet ./...
gofmt -l .
```

## Архитектура

- `config.go` - разбор аргументов и окружения.
- `api.go` - последовательные HTTP-запросы, пагинация по `Link`, фильтрация, дедупликация, повторы.
- `cache.go` - дисковый кэш условных запросов.
- `trace.go` - журнал JSON Lines.
- `report.go`, `main.go` - схема отчёта, коды возврата, вывод.

Запросы: `GET {base}/repos/{owner}/{repo}/issues?state=open&sort=created&direction=asc&per_page=100`.
Заголовки: `Accept`, `User-Agent: ghreport/1.0`, `X-GitHub-Api-Version: 2026-03-10`, `Authorization` при наличии токена.
Следующая страница берётся только из `Link: rel="next"`.

## Кэш

Каталог `--cache-dir` создаётся автоматически. Для каждого адреса хранится файл `<sha256(url)>.json`:

```json
{"etag": "\"abc\"", "body": "[...]", "link": "<...>; rel=\"next\""}
```

При наличии `ETag` отправляется `If-None-Match`. Ответ `304` использует тело из кэша, `200` обновляет кэш. Токен не сохраняется.

## Лимиты и ошибки

- Сеть и `5xx`: до трёх повторов с паузами 1, 2 и 4 секунды.
- `403`/`429`: пауза по `Retry-After`, затем по `X-RateLimit-Reset`, иначе 60 секунд; до трёх повторов.
- `401`: `{"error": "unauthorized"}`, код 3.
- Ошибки репозитория: `not_found`, `rate_limited`, `request_failed`, `invalid_response`.
- Часть репозиториев с ошибкой: отчёт и код 1. Все с ошибкой: `{"error": "all_failed"}`, код 3.

## Пример

```json
{"generated_at":"2026-10-14T09:00:00Z","repos":[{"repo":"cli/cli","error":null,"count":1,"issues":[{"number":1,"title":"Первый выпуск","author":"octocat","labels":["bug"],"created_at":"2026-01-05T10:00:00Z","url":"https://github.com/cli/cli/issues/1"}]}]}
```

Журнал:

```json
{"ts":"2026-10-14T09:00:01Z","method":"GET","url":"https://api.github.com/repos/cli/cli/issues?state=open&sort=created&direction=asc&per_page=100","status":200,"attempt":1,"from_cache":false,"error":null}
```
