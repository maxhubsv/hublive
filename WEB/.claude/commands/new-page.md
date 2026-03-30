Create a new page named $ARGUMENTS.

Steps:
1. Create src/pages/${ARGUMENTS}Page.tsx with default export
2. Add route in App.tsx
3. If requires auth: wrap in ProtectedRoute
4. If requires admin: check role === 'admin', redirect if not
5. Add page title via useTranslation
6. Add ErrorBoundary wrapper
7. Add loading state (Suspense + Skeleton)

Template:
```tsx
import { useTranslation } from 'react-i18next';
import { ErrorBoundary } from '@/components/shared/ErrorBoundary';

export default function ${ARGUMENTS}Page() {
  const { t } = useTranslation();
  return (
    <ErrorBoundary>
      <div className="p-6">
        <h1 className="text-xl font-semibold">{t('page.title')}</h1>
      </div>
    </ErrorBoundary>
  );
}
```
