Review all modified files in the current branch.

Check each file against these criteria:

1. TypeScript: no `any`, proper interfaces, strict types
2. Components: functional only, props typed, hooks extracted, cn() for classes
3. i18n: ALL visible text via t(), keys exist in all 3 locale files (vi, en, zh)
4. State: Zustand for client, React Query for server, no mixing
5. API: through client.ts, typed request/response, error handled with toast
6. Styling: Tailwind classes, no inline styles, no hardcoded colors
7. Accessibility: alt on images, aria-label on icon buttons, keyboard nav
8. Performance: no unnecessary re-renders, selectors on Zustand, lazy loading pages
9. Error handling: ErrorBoundary at route, onError on mutations, no swallowed catches

For each issue found, report:
- File path
- Line number
- Severity: ERROR (must fix) / WARN (should fix)
- Description
- Suggested fix

After review, run:
- npx tsc --noEmit
- npm run build
- Report any errors.
