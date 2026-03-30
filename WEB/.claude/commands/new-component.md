Create a new React component named $ARGUMENTS.

Follow the component-pattern skill exactly:
1. Create src/components/[appropriate-folder]/$ARGUMENTS.tsx
2. Props interface named ${ARGUMENTS}Props
3. Named export
4. All text via t() from react-i18next
5. className via cn() helper
6. Add keyboard accessibility (role, tabIndex, onKeyDown for interactive elements)

If the component needs data, create a corresponding hook in src/hooks/.
If the component needs API calls, use React Query pattern from api-pattern skill.
