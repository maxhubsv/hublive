---
name: api-pattern
description: Kích hoạt khi tạo/sửa API calls, hooks fetch data, hoặc file trong src/api/. Đảm bảo mọi API call đúng pattern: React Query + Axios + typed responses.
---

# API Pattern

## Quy tắc
- MỌI API call qua `src/api/client.ts` (Axios instance). KHÔNG fetch() trực tiếp.
- MỌI data fetching qua React Query `useQuery`. KHÔNG useEffect + fetch.
- MỌI data mutation qua React Query `useMutation`.
- MỌI types trong `src/api/types.ts`. KHÔNG duplicate type ở nơi khác.
- MỌI error → toast notification.
- MỌI loading → skeleton hoặc spinner.

## Cấu trúc file API
```typescript
// src/api/[domain].api.ts
import client from './client';
import type { CreateSchoolRequest, SchoolResponse } from './types';

export const schoolsApi = {
  getAll: () => client.get<SchoolResponse[]>('/api/v1/schools').then(r => r.data),
  getById: (id: string) => client.get<SchoolResponse>(`/api/v1/schools/${id}`).then(r => r.data),
  create: (data: CreateSchoolRequest) => client.post<SchoolResponse>('/api/v1/schools', data).then(r => r.data),
  update: (id: string, data: CreateSchoolRequest) => client.put<SchoolResponse>(`/api/v1/schools/${id}`, data).then(r => r.data),
  remove: (id: string) => client.delete(`/api/v1/schools/${id}`),
};
```

## Cấu trúc hook
```typescript
// src/hooks/useSchools.ts
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { schoolsApi } from '@/api/schools.api';
import { toast } from 'sonner';
import { useTranslation } from 'react-i18next';

export function useSchools() {
  return useQuery({
    queryKey: ['schools'],
    queryFn: schoolsApi.getAll,
    staleTime: 30_000,
  });
}

export function useCreateSchool() {
  const qc = useQueryClient();
  const { t } = useTranslation();
  return useMutation({
    mutationFn: schoolsApi.create,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['schools'] });
      toast.success(t('admin.school.created'));
    },
    onError: (err: Error) => toast.error(err.message),
  });
}
```

## KHÔNG LÀM
```typescript
// ❌ fetch trực tiếp
useEffect(() => { fetch('/api/schools').then(r => r.json()).then(setData) }, []);

// ❌ axios import trực tiếp (bypass interceptors)
import axios from 'axios';
axios.get('/api/schools');

// ❌ type inline thay vì từ types.ts
const data: { id: string; name: string }[] = await resp.json();

// ❌ silent error
try { await api.create(data); } catch {} // swallowed!
```
