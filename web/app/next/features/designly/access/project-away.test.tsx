// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import ProjectAway from './project-away';
import { AWAY } from '@/next/features/designly/project-read';
import { t } from '@/next/i18n';
afterEach(cleanup);

it('names the reason and the owner, and leads back', () => {
  const onBack = vi.fn();
  render(<ProjectAway why={AWAY.silentNoCopy} ownerName="Анна" onBack={onBack} />);
  const card = screen.getByRole('status');
  expect(card.textContent).toContain(t('next.access.away.silentNoCopy.title'));
  expect(card.textContent).toContain(t('next.access.owner', { name: 'Анна' }));
  fireEvent.click(screen.getByRole('button', { name: t('next.access.back') }));
  expect(onBack).toHaveBeenCalled();
});
