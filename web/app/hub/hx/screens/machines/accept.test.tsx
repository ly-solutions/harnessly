// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { OrgMachine } from '../../../org/api/machines';

import { AcceptMachine } from './accept';

const machine = (more: Partial<OrgMachine>) =>
  ({ id: 'm1', name: 'build-1', ownerName: 'Bob', mine: true, ...more }) as unknown as OrgMachine;

afterEach(() => cleanup());

describe('A machine that waits for its owner', () => {
  it('says nothing about a machine that does not wait', () => {
    const { container } = render(<AcceptMachine m={machine({ awaitsOwner: null })} onAccept={vi.fn()} />);
    expect(container.textContent).toBe('');
  });

  it('tells the owner who may still control it, and accepts only on the second press', async () => {
    const onAccept = vi.fn(async () => true);
    render(<AcceptMachine m={machine({ awaitsOwner: { heldBy: 'Ann' } })} onAccept={onAccept} />);
    fireEvent.click(screen.getByRole('button', { name: 'Принять машину' }));
    expect(onAccept).not.toHaveBeenCalled();
    expect(screen.getByText(/Подключал её Ann: он может по-прежнему управлять машиной/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Да, принять' }));
    await waitFor(() => expect(onAccept).toHaveBeenCalledOnce());
  });

  it('offers no button to anybody but the owner', () => {
    render(<AcceptMachine m={machine({ mine: false, awaitsOwner: { heldBy: 'Ann' } })} onAccept={vi.fn()} />);
    expect(screen.getByText(/ждёт, когда её примет владелец/)).toBeTruthy();
    expect(screen.queryByRole('button')).toBeNull();
  });
});
