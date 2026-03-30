---
name: i18n-rules
description: Kích hoạt khi viết UI text, tạo components hiển thị cho user, hoặc chỉnh sửa locale files. Đảm bảo mọi text qua t() và đồng bộ tất cả locale files.
---

# i18n Rules

## Quy tắc tuyệt đối
- MỌI text hiển thị cho user PHẢI qua `t()`. KHÔNG hardcode string.
- Khi thêm key mới: thêm vào TẤT CẢ locale files (vi.json, en.json, zh.json).
- Key format: dot notation, group theo feature: `auth.login`, `grid.selectAll`.
- Placeholder: `{{variable}}` syntax: `t('grid.selected', { count: 5 })`.
- Plurals: `_one` / `_other` suffix.

## Usage
```tsx
import { useTranslation } from 'react-i18next';

function MyComponent() {
  const { t } = useTranslation();
  
  return (
    <div>
      <h1>{t('page.title')}</h1>
      <p>{t('page.description', { name: userName })}</p>
      <span>{t('grid.computerCount', { count: total })}</span>
    </div>
  );
}
```

## Key naming convention
```
app.*           — global: loading, error, save, cancel, confirm
auth.*          — authentication: login, logout, errors
sidebar.*       — navigation sidebar
header.*        — top header bar
grid.*          — computer grid view
computer.*      — computer info and states
feature.*       — feature controls (lock, message, power, demo)
admin.*         — admin panel CRUD
stream.*        — WebRTC streaming status
time.*          — relative time formatting
validation.*   — form validation messages
```

## KHÔNG LÀM
```tsx
// ❌ Hardcode text
<Button>Đăng nhập</Button>
<p>Không có dữ liệu</p>
<span>3 máy đã chọn</span>

// ✅ Qua t()
<Button>{t('auth.loginButton')}</Button>
<p>{t('app.noData')}</p>
<span>{t('grid.selected', { count: 3 })}</span>
```

## Khi thêm key mới — BẮT BUỘC cập nhật cả 3 files
```
src/i18n/locales/vi.json  ← Tiếng Việt
src/i18n/locales/en.json  ← English
src/i18n/locales/zh.json  ← 中文
```
Thiếu 1 file = bug hiển thị key thô cho user.
