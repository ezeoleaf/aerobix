# Contributing

Thanks for helping improve Aerobix.

## Development Setup

```bash
go mod tidy
make run
```

## Suggested Workflow

1. Create a branch from `main`.
2. Keep PRs small and focused.
3. Include rationale in commit messages.
4. Run before opening a PR:

```bash
make fmt
make test
make lint
make build
```

## Code Guidelines

- Keep provider-specific code isolated in `provider/<name>`.
- Keep physiological calculations in `physics`.
- Keep UI logic in `ui` (Bubble Tea MVU style).
- Prefer explicit error handling and deterministic behavior for metrics.

## High-Value Contribution Areas

- Garmin provider adapter
- Better run scoring (GAP-based effort model)
- User-custom HR/power zones
- Tests for metrics and parsing edge cases
- FIT/TCX import pipeline with concurrent parsing
