# WCAP Web Dashboard - Design Spec

## Overview

Web dashboard for the WCAP screen capture/streaming system. Provides a full admin panel to manage agents, view live streams, and control remote desktops. Targets web PC and web mobile browsers.

**Starting point**: Minimal structure + agent test screen. Auth, i18n, admin CRUD, and advanced features will be added incrementally.

## Tech Stack

| Layer | Technology | Version |
|-------|-----------|---------|
| Framework | React + TypeScript | 19.x + 5.9.x |
| Build | Vite | 8.x |
| State (client) | Zustand | 5.x |
| State (server) | TanStack React Query | 5.x |
| Styling | Tailwind CSS v4 + shadcn/ui | 4.x |
| Routing | React Router | v7 |
| HTTP Client | Axios | 1.x |
| Icons | Lucide React | latest |
| Toasts | Sonner | 2.x |
| Streaming | hublive-client | latest |
| Date | date-fns | 4.x |

## Reference

Based on hub32-dashboard (C:\Users\Admin\Desktop\veyon\hub32-dashboard) architecture. Same patterns for:
- Zustand stores with localStorage persistence
- Axios client with JWT interceptors (401 → logout)
- shadcn/ui component library with radix-nova style
- Dark theme with CSS variables and pre-flash prevention
- AppLayout with collapsible sidebar + header + outlet
- ProtectedRoute wrapper
- ErrorBoundary

## Project Structure

```
WEB/
├── index.html                        # Entry (dark theme pre-flash script)
├── package.json
├── vite.config.ts                    # Vite + React + Tailwind + @/ alias
├── tsconfig.json                     # Base config with path aliases
├── tsconfig.app.json                 # App-specific strict TS config
├── components.json                   # shadcn/ui configuration
├── src/
│   ├── main.tsx                      # ReactDOM.createRoot entry
│   ├── App.tsx                       # Routes + QueryClient + Toaster
│   ├── index.css                     # Tailwind + dark theme CSS variables
│   │
│   ├── api/
│   │   ├── client.ts                 # Axios instance + auth interceptors
│   │   ├── types.ts                  # API DTOs and response types
│   │   └── agents.api.ts             # Agent CRUD + status endpoints
│   │
│   ├── stores/
│   │   ├── auth.store.ts             # JWT auth (token, user, login, logout)
│   │   └── theme.store.ts            # Theme persistence (dark default)
│   │
│   ├── hooks/
│   │   └── useStream.ts              # HubLive room connection + track subscription
│   │
│   ├── pages/
│   │   ├── LoginPage.tsx             # Login form
│   │   └── DashboardPage.tsx         # Agent grid + stream viewer
│   │
│   ├── components/
│   │   ├── layout/
│   │   │   ├── AppLayout.tsx         # Sidebar + Header + Outlet
│   │   │   ├── AppSidebar.tsx        # Collapsible/pinnable sidebar
│   │   │   └── Header.tsx            # Top bar with user menu
│   │   ├── agent/
│   │   │   ├── AgentGrid.tsx         # Responsive grid of agent cards
│   │   │   └── AgentCard.tsx         # Agent status + preview thumbnail
│   │   ├── stream/
│   │   │   └── ScreenViewer.tsx      # HubLive video track renderer
│   │   ├── shared/
│   │   │   ├── ProtectedRoute.tsx    # Auth route guard (Outlet-based)
│   │   │   ├── ErrorBoundary.tsx     # React error boundary
│   │   │   └── Spinner.tsx           # Loading indicator
│   │   └── ui/                       # shadcn/ui auto-generated primitives
│   │
│   ├── lib/
│   │   ├── utils.ts                  # cn() helper (clsx + tailwind-merge)
│   │   └── constants.ts              # API URLs, config
│   │
│   └── types/
│       └── index.ts                  # Shared app types
```

## Routing

```
/login          → LoginPage (public)
/dashboard      → DashboardPage (protected, default after login)
/*              → Redirect to /dashboard
```

Future routes: `/admin`, `/settings`, `/agents/:id`

## Key Components

### DashboardPage (Agent Test Screen)
- Fetches agent list from API (or mock data initially)
- Renders AgentGrid with responsive columns:
  - Desktop (≥1280px): 4 columns
  - Tablet (≥768px): 2 columns
  - Mobile (<768px): 1 column
- Click agent card → expand ScreenViewer with HubLive stream
- Agent status indicators: online (green), offline (gray), streaming (blue pulse)
- Display metadata: hostname, IP, resolution, FPS, uptime

### ScreenViewer (HubLive Integration)
- Uses `hublive-client` SDK to connect to HubLive room
- Subscribe to agent's video track
- Render via `<video>` element with HubLive's `attach()` method
- Connection states: connecting, connected, reconnecting, disconnected
- Controls: fullscreen, quality selector (if available)

### Auth Flow
- JWT token stored in localStorage
- Axios request interceptor attaches Bearer token
- Axios response interceptor: 401 → clear auth → redirect /login
- Zustand auth store initializes synchronously from localStorage (no flash)

## Styling

### Dark Theme (Default)
CSS variable-based theming matching hub32-dashboard pattern:
- Background: slate/zinc dark tones
- Text: white/gray hierarchy
- Accent: brand color (configurable)
- Status colors: green (online), gray (offline), blue (streaming), red (error)

### Responsive Design
- Mobile-first Tailwind breakpoints
- Sidebar: hidden on mobile, collapsible on tablet, pinnable on desktop
- Touch-friendly tap targets (min 44px) on mobile

## HubLive Integration

### Dependencies
- `hublive-client`: Core SDK for room connection and track subscription
- Server-side: Go backend generates JWT tokens for HubLive room access

### Stream Flow
1. User clicks agent card
2. Frontend requests HubLive token from Go backend (passing agent room name)
3. `useStream` hook creates `Room` instance, connects with token
4. Subscribes to remote participant's video track
5. Attaches track to `<video>` element
6. Handles reconnection and cleanup on unmount

## Future Additions (Not in initial scope)
- [ ] Authentication (login/register forms)
- [ ] i18n (vi/en/zh)
- [ ] Admin panel (agent management CRUD)
- [ ] Remote control features (lock, power, message)
- [ ] Testing (Vitest + MSW)
- [ ] Multi-theme support
- [ ] Audit logging
