# es-AR operator override layer (Batch F4c)

## Merge semantics

Runtime lookup (`getTranslation` / `SimpleTranslationProvider`) builds:

```
esArTree = deepMerge(deepMerge(esTree, messages/es-ar/*), { common: locales/es-ar/common })
```

So `messages/es-ar/` is a **thin overlay**, not a standalone complete copy.
Missing keys inherit Peninsular `es`. Operator parity validator treats es-AR as
an override set (extra keys forbidden; missing keys allowed).

## Identical keys (deadweight)

A 2026-07-09 scan found ~156 es-ar leaf values **byte-identical** to es and
~1380 that differ. Identical leaves have no runtime effect under deepMerge.
They are optional hygiene debt — **not** pruned in Batch F because:

1. Bulk deletes risk fighting the Rioplatense assembler / style-guide workflow.
2. Some identical keys may be intentional placeholders pending copy review.
3. Removing them does not change user-visible strings.

Prefer a dedicated prune script with dry-run + native review if/when cleaning.

## Related maps

- There is no `es-AR → es` collapse map any more. Operator chrome strings
  resolve through `getTranslation` with the raw locale, which applies this
  override layer on top of `es`.
