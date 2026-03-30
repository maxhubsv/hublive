---
name: testing-pattern
description: Kích hoạt khi viết tests, tạo file .test.tsx, hoặc khi được yêu cầu test component/hook. Đảm bảo React Testing Library + user-centric testing.
---

# Testing Pattern

## Tool: Vitest + React Testing Library
```bash
npm run test           # run all tests
npm run test -- --watch  # watch mode
```

## Quy tắc
- Test behavior, KHÔNG test implementation.
- Query bằng role/label/text, KHÔNG bằng class/id/data-testid (trừ khi bắt buộc).
- Mỗi test = 1 user action + 1 expected result.
- File test cạnh source: `ComputerCard.tsx` → `ComputerCard.test.tsx`
- Test name mô tả: `it('shows online badge when computer state is online')`

## Template
```tsx
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ComputerCard } from './ComputerCard';

const mockComputer = { id: '1', name: 'PC-01', hostname: 'pc01', location: 'loc-1', state: 'online' as const };

describe('ComputerCard', () => {
  it('renders computer name', () => {
    render(<ComputerCard computer={mockComputer} isSelected={false} onSelect={vi.fn()} onClick={vi.fn()} />);
    expect(screen.getByText('PC-01')).toBeInTheDocument();
  });

  it('calls onClick when card is clicked', async () => {
    const onClick = vi.fn();
    render(<ComputerCard computer={mockComputer} isSelected={false} onSelect={vi.fn()} onClick={onClick} />);
    await userEvent.click(screen.getByRole('button'));
    expect(onClick).toHaveBeenCalledWith('1');
  });

  it('shows selected style when isSelected is true', () => {
    const { container } = render(<ComputerCard computer={mockComputer} isSelected={true} onSelect={vi.fn()} onClick={vi.fn()} />);
    expect(container.firstChild).toHaveClass('selected');
  });
});
```

## KHÔNG LÀM
```tsx
// ❌ Test implementation details
expect(component.state.isOpen).toBe(true);

// ❌ Query bằng class
document.querySelector('.computer-card');

// ❌ Snapshot test thay behavior test
expect(tree).toMatchSnapshot();
```
