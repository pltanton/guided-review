# Hotspots

A hotspot is a place where a mistake is expensive. Mark it on the step with a
category, a concrete question, and the line when there is one.

| cat         | look for                                                                                     |
| ----------- | -------------------------------------------------------------------------------------------- |
| security    | authn/authz checks, input validation, secrets or PII in logs, injections, SSRF               |
| consistency | transaction boundaries, races and double spend, idempotency, outbox and event order, retries |
| money       | rounding, float vs decimal, currency and scale, sign of amounts, limits                      |
| migration   | locks on large tables, backfill, backward compatibility with the running version             |

With `domain: finance` in `.review.yaml` the bar is higher: every change that moves,
reserves or displays money gets a `money` hotspot.

Ask, do not assert. The question must be answerable by reading the code:

- good: «Повтор с тем же idempotency key после таймаута — второе списание?»
- good: «Сумма в float64 до записи в БД — где округление?»
- bad: «Тут может быть гонка» (no scenario)
- bad: «Проверьте безопасность» (no question)

A hotspot is never boilerplate and never goes stale automatically; only the human
drops it.
