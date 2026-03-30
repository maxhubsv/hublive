---
name: error-handling
description: Kích hoạt khi viết try/catch, error boundaries, toast notifications, hoặc xử lý lỗi API. Đảm bảo mọi error được catch và hiển thị cho user.
---

# Error Handling

## Layers
1. **ErrorBoundary** — catch React render errors, wrap ở route level
2. **React Query onError** — catch API errors trong mutations
3. **Axios interceptor** — catch 401 → redirect login
4. **Form validation** — inline errors dưới mỗi field
5. **Toast** — thông báo success/error cho user actions

## Template ErrorBoundary
```tsx
import { Component, ReactNode } from 'react';

interface Props { children: ReactNode; fallback?: ReactNode; }
interface State { hasError: boolean; error?: Error; }

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false };
  static getDerivedStateFromError(error: Error) { return { hasError: true, error }; }
  render() {
    if (this.state.hasError) {
      return this.props.fallback || (
        <div className="flex flex-col items-center justify-center h-64 gap-4">
          <p className="text-red-400">Đã xảy ra lỗi</p>
          <button onClick={() => this.setState({ hasError: false })}>Thử lại</button>
        </div>
      );
    }
    return this.props.children;
  }
}
```

## Mutation error toast
```typescript
useMutation({
  mutationFn: api.create,
  onError: (err: Error) => {
    toast.error(err.message || t('app.error'));
  },
});
```

## KHÔNG LÀM
```typescript
// ❌ Swallow error
try { await api.delete(id); } catch {}

// ❌ console.error only (user không thấy)
catch (err) { console.error(err); }

// ❌ alert() thay toast
catch (err) { alert(err.message); }
```
