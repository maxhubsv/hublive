---
name: component-pattern
description: Kích hoạt khi tạo/sửa React components (.tsx). Đảm bảo functional components, typed props, hooks tách riêng, cn() cho class names.
---

# Component Pattern

## Quy tắc
- Functional components ONLY. KHÔNG class components.
- Props interface ngay trên component, cùng file.
- Destructure props trong function signature.
- Custom hooks cho business logic. Component chỉ UI + hooks.
- `cn()` cho conditional classNames. KHÔNG string concatenation.
- Mỗi component 1 file. File name = component name.
- Named export cho shared, default export cho pages.

## Template chuẩn
```tsx
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/cn';

interface FeatureCardProps {
  title: string;
  isActive: boolean;
  onClick: () => void;
  className?: string;
}

export function FeatureCard({ title, isActive, onClick, className }: FeatureCardProps) {
  const { t } = useTranslation();

  return (
    <div
      className={cn(
        "rounded-md border p-4 cursor-pointer transition-colors",
        isActive ? "border-blue-500 bg-blue-500/10" : "border-zinc-800",
        className
      )}
      onClick={onClick}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => e.key === 'Enter' && onClick()}
    >
      <h3 className="text-sm font-medium">{title}</h3>
      <span className="text-xs text-zinc-400">
        {isActive ? t('app.active') : t('app.inactive')}
      </span>
    </div>
  );
}
```

## Hooks extraction
```tsx
// ❌ Business logic trong component
function UserList() {
  const [users, setUsers] = useState([]);
  const [filter, setFilter] = useState('');
  const filtered = users.filter(u => u.name.includes(filter));
  // ... 30 dòng logic
  return <div>...</div>;
}

// ✅ Logic tách vào hook
function UserList() {
  const { users, filter, setFilter, isPending } = useUsers();
  return <div>...</div>;
}
```

## Checklist trước commit
- [ ] Props có interface/type
- [ ] Không any trong props
- [ ] Text qua t() (nếu có i18n)
- [ ] className dùng cn()
- [ ] Keyboard accessible (role, tabIndex, onKeyDown)
- [ ] Loading state handled
- [ ] Error state handled
