---
name: state-pattern
description: Kích hoạt khi tạo/sửa stores, state management, hoặc data flow giữa components. Đảm bảo Zustand cho client state, React Query cho server state.
---

# State Management Pattern

## Phân chia rõ ràng
| Loại | Tool | Ví dụ |
|---|---|---|
| Client state | Zustand | auth, UI selections, sidebar open/close |
| Server state | React Query | computers list, schools, teachers |
| Form state | React Hook Form hoặc local useState | input values, validation |
| URL state | React Router | current page, query params |

## Zustand store template
```typescript
import { create } from 'zustand';

interface RoomState {
  selectedLocationId: string | null;
  selectedComputerIds: Set<string>;
  // Actions
  selectLocation: (id: string) => void;
  toggleComputer: (id: string) => void;
  selectAll: (ids: string[]) => void;
  deselectAll: () => void;
}

export const useRoomStore = create<RoomState>((set) => ({
  selectedLocationId: null,
  selectedComputerIds: new Set(),
  
  selectLocation: (id) => set({ selectedLocationId: id, selectedComputerIds: new Set() }),
  toggleComputer: (id) => set((s) => {
    const next = new Set(s.selectedComputerIds);
    next.has(id) ? next.delete(id) : next.add(id);
    return { selectedComputerIds: next };
  }),
  selectAll: (ids) => set({ selectedComputerIds: new Set(ids) }),
  deselectAll: () => set({ selectedComputerIds: new Set() }),
}));
```

## Selector pattern (tránh re-render)
```typescript
// ✅ Select chỉ field cần — component chỉ re-render khi field này thay đổi
const locationId = useRoomStore((s) => s.selectedLocationId);
const count = useRoomStore((s) => s.selectedComputerIds.size);

// ❌ KHÔNG lấy toàn bộ store — re-render khi BẤT KỲ field nào thay đổi
const store = useRoomStore();
```

## KHÔNG LÀM
```typescript
// ❌ Server data trong Zustand (React Query quản lý cache/refetch tốt hơn)
const useStore = create((set) => ({
  computers: [],
  fetchComputers: async () => {
    const data = await api.getComputers();
    set({ computers: data });
  }
}));

// ❌ React Context cho global state (performance kém, re-render toàn tree)
const AppContext = createContext({ user: null, theme: 'dark' });

// ❌ Prop drilling qua 4+ levels (dùng Zustand hoặc React Query)
<A user={user}> → <B user={user}> → <C user={user}> → <D user={user}>
```
