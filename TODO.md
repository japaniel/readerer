# TODO

- Initialize Go module and basic package
- Add unit tests for `pkg/readerer`
- Add CI (GitHub Actions)
- Add README usage examples
- Support reviewing words by source (associate words with multiple sources via a join table)

## Future enhancements (deferred)

- Add similar-word suggestions (deferred)
  - Option 1 (fast): similarity = same lemma or POS — SELECT other words with same lemma from other sources.
  - Option 2 (advanced): semantic similarity via embedding index (future feature).

- Add semantic search / embeddings for improved discovery and clustering of words across sources.
