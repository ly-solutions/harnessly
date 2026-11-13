// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { OrgMachine } from '../../../org/api/machines';

import { Owner } from './dialogs';

const machine = {
  id: 'm1',
  name: 'build-1',
  ownerId: 'u1',
  ownerName: 'Ann',
  canAdmin: true,
} as unknown as OrgMachine;
const people = [
  { id: 'u1', name: 'Ann' },
  { id: 'u2', name: 'Bob' },
];

afterEach(() => cleanup());

describe('Owner dialog of the machine card', () => {
  it('does not offer the current owner and hands over only after a choice', async () => {
    const onConfirm = vi.fn(async () => true);
    render(<Owner m={machine} people={people} onClose={vi.fn()} onConfirm={onConfirm} />);
    expect(screen.queryByRole('option', { name: 'Ann' })).toBeNull();
    fireEvent.change(screen.getByLabelText('Кому передать'), { target: { value: 'u2' } });
    await screen.findByText(/Владелец Ann сменится на Bob/);
    fireEvent.click(screen.getByRole('button', { name: 'Передать' }));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledWith('u2', 'move'));
  });

  it('sends the history choice the person made', async () => {
    const onConfirm = vi.fn(async () => true);
    render(<Owner m={machine} people={people} onClose={vi.fn()} onConfirm={onConfirm} />);
    fireEvent.change(screen.getByLabelText('Кому передать'), { target: { value: 'u2' } });
    fireEvent.click(screen.getByRole('radio', { name: 'Оставить прежнему владельцу' }));
    fireEvent.click(screen.getByRole('button', { name: 'Передать' }));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledWith('u2', 'keep'));
  });
});
