---
name: styling-pattern
description: Kích hoạt khi viết CSS, Tailwind classes, theme, hoặc layout. Đảm bảo Tailwind + shadcn/ui + cn() + design system variables.
---

# Styling Pattern

## Ưu tiên (từ cao → thấp)
1. shadcn/ui component → dùng trước nếu có
2. Tailwind utility → dùng cho layout, spacing, color
3. CSS variable → dùng cho design system tokens
4. Custom CSS → CHỈ khi Tailwind không có (animation keyframes, scrollbar)

## cn() bắt buộc cho conditional classes
```typescript
import { cn } from '@/lib/cn';

// ✅
<div className={cn("p-4 rounded-md", isActive && "bg-blue-500/10", className)} />

// ❌ String concatenation
<div className={`p-4 rounded-md ${isActive ? 'bg-blue-500/10' : ''}`} />

// ❌ Inline style thay Tailwind
<div style={{ padding: '16px', borderRadius: '6px' }} />
```

## Design system (CSS variables trong index.css)
```css
var(--bg-primary)      /* #0A0A0B — main background */
var(--bg-secondary)    /* #141416 — card/panel */
var(--bg-tertiary)     /* #1C1C1F — hover */
var(--text-primary)    /* #FAFAFA — white text */
var(--text-secondary)  /* #A0A0A8 — muted */
var(--accent)          /* #3B82F6 — blue */
var(--success)         /* #22C55E — online */
var(--danger)          /* #EF4444 — error/offline */
var(--warning)         /* #F59E0B — connecting */
var(--font-sans)       /* IBM Plex Sans */
var(--font-mono)       /* JetBrains Mono — data display */
```

## Responsive breakpoints
```
sm:  640px    — mobile landscape
md:  768px    — tablet portrait
lg:  1024px   — tablet landscape (minimum support)
xl:  1280px   — desktop
2xl: 1536px   — wide desktop
```

## KHÔNG LÀM
- KHÔNG CSS modules (.module.css)
- KHÔNG styled-components / emotion / CSS-in-JS
- KHÔNG !important
- KHÔNG magic pixel values — dùng Tailwind spacing scale (p-2, p-4, gap-3)
- KHÔNG hardcode color hex trong className — dùng Tailwind colors hoặc CSS variables
