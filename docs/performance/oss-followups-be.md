# Pre-release backend follow-ups: benchmark log

## Basic-mode waiter menu-listing intents

Change: `classifyWaiterV2Intent` recognises more full-menu phrasings
("show me the menu", "what's on the menu", "what do you have", ...) and
`WaiterMenuSnapshot.MatchCategoriesInText` accepts a simple English plural of a
category name ("pizzas" → "Pizza"). Both run once per guest waiter turn.

Benchmark: `BenchmarkClassifyWaiterV2Intent_Unmatched` (worst case: a turn
that walks the whole intent ladder), local SQLite-free microbenchmark.

```
cd backend && go test -run '^$' -bench BenchmarkClassifyWaiterV2Intent_Unmatched -benchmem -count=3 ./internal/server/
```

| | ns/op | B/op | allocs/op |
|---|---|---|---|
| before | 16.6k–18.1k | 14,296 | 213 |
| after  | 18.7k–19.0k | 15,192 | 238 |

About +1.5 µs and +25 allocs per turn, from the extra phrase and plural
checks. No database access changed. Accepted: the turn is otherwise dominated
by snapshot loading and (in LLM mode) the model call.
