// «Макет → Настройки»: the settings the open board offers (its data-props, the screen state and theme of a
// wrapper board), one compact row each, «label — control»: a few short options are a segment, many or long ones a list
// of our own, the steps of a scenario ‹ ›, a switch a toggle, colours dots. A dot marks a setting away from the board's
// own; «Вернуть как на доске» puts them all back. What is picked is everybody's who looks at the project and the
// agent's (meta/props.json, use-site-props).
import { Fragment, useLayoutEffect, useRef, useState } from 'react';
import type { ITweak, TTweakValue } from '@/next/features/designly/page-bridge';
import {
  rangeShare,
  stepOption,
  tweakChanged,
  tweakGroups,
  tweakKind,
  tweakValueText,
  TWEAK_KIND,
} from '@/next/features/designly/tweaks';
import Icon from '@/next/ui/icon';
import { t } from '@/next/i18n';

interface IControlProps {
  tweak: ITweak;
  onChange: (value: TTweakValue) => void;
}

const Seg = ({ tweak, onChange }: IControlProps) => (
  <div className="hx-ds-seg" role="group" aria-label={tweak.label}>
    {tweak.options.map((o) => (
      <button
        key={String(o.value)}
        type="button"
        aria-pressed={o.value === tweak.value}
        onClick={() => o.value !== tweak.value && onChange(o.value)}
      >
        {o.text}
      </button>
    ))}
  </div>
);

const Dot = ({
  color,
  on,
  label,
  onPick,
}: {
  color: string;
  on: boolean;
  label: string;
  onPick: () => void;
}) => {
  const ref = useRef<HTMLButtonElement>(null);
  useLayoutEffect(() => {
    ref.current?.style.setProperty('--c', color);
  }, [color]);
  return (
    <button
      ref={ref}
      type="button"
      className="hx-ds-dot"
      aria-pressed={on}
      aria-label={label}
      title={label}
      onClick={onPick}
    />
  );
};

const Dots = ({ tweak, onChange }: IControlProps) => (
  <div className="hx-ds-dots">
    {tweak.options.map((o) => (
      <Dot
        key={String(o.value)}
        color={String(o.value)}
        label={o.text}
        on={o.value === tweak.value}
        onPick={() => o.value !== tweak.value && onChange(o.value)}
      />
    ))}
  </div>
);

/** A list of our own (no native select): the value, and the options under it. */
export const Dropdown = ({ tweak, onChange }: IControlProps) => {
  const [open, setOpen] = useState(false);
  return (
    <div
      className="hx-ds-dd"
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false);
      }}
      onKeyDown={(event) => {
        if (event.key === 'Escape') setOpen(false);
      }}
    >
      <button
        type="button"
        className="hx-ds-dd__btn"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={tweak.label}
        onClick={() => setOpen((on) => !on)}
      >
        <span>{tweakValueText(tweak)}</span>
        <Icon name="chevron-down" size="sm" />
      </button>
      {open && (
        <div className="hx-ds-pop" role="listbox">
          {tweak.options.map((o) => (
            <button
              key={String(o.value)}
              type="button"
              role="option"
              className="hx-ds-pop__i"
              aria-selected={o.value === tweak.value}
              onClick={() => {
                setOpen(false);
                if (o.value !== tweak.value) onChange(o.value);
              }}
            >
              <span>{o.text}</span>
              {o.value === tweak.value && <Icon name="check" size="sm" />}
            </button>
          ))}
        </div>
      )}
    </div>
  );
};

const Step = ({ tweak, onChange }: IControlProps) => (
  <div className="hx-ds-step">
    <button
      type="button"
      aria-label={t('next.ds.prev')}
      onClick={() => onChange(stepOption(tweak, -1).value)}
    >
      <Icon name="chevron-left" size="sm" />
    </button>
    <b>{tweakValueText(tweak)}</b>
    <button type="button" aria-label={t('next.ds.next')} onClick={() => onChange(stepOption(tweak, 1).value)}>
      <Icon name="chevron-right" size="sm" />
    </button>
  </div>
);

const Range = ({ tweak, onChange }: IControlProps) => {
  const ref = useRef<HTMLInputElement>(null);
  const [held, setHeld] = useState<number | null>(null);
  const min = tweak.min ?? 0;
  const max = tweak.max ?? 0;
  const value = held ?? (typeof tweak.value === 'number' ? tweak.value : min);
  useLayoutEffect(() => {
    ref.current?.style.setProperty('--p', rangeShare(value, min, max));
  }, [value, min, max]);
  return (
    <div className="hx-ds-range">
      <input
        ref={ref}
        type="range"
        min={min}
        max={max}
        step={tweak.step ?? 1}
        value={value}
        aria-label={tweak.label}
        onChange={(event) => {
          const next = Number(event.currentTarget.value);
          setHeld(next);
          onChange(next);
        }}
        onPointerUp={() => setHeld(null)}
        onBlur={() => setHeld(null)}
      />
      <span>{value}</span>
    </div>
  );
};

const Field = ({ tweak, onChange, numeric }: IControlProps & { numeric: boolean }) => {
  const [typed, setTyped] = useState<string | null>(null);
  const shown = typed ?? (tweak.value === null ? '' : String(tweak.value));
  const commit = () => {
    if (typed === null) return;
    if (!numeric) onChange(typed);
    else if (typed.trim() !== '' && Number.isFinite(Number(typed))) onChange(Number(typed));
    setTyped(null);
  };
  return (
    <input
      className="hx-ds-field"
      type={numeric ? 'number' : 'text'}
      value={shown}
      aria-label={tweak.label}
      autoComplete="off"
      onChange={(event) => setTyped(event.currentTarget.value)}
      onKeyDown={(event) => {
        if (event.key === 'Enter') commit();
      }}
      onBlur={commit}
    />
  );
};

const Control = ({ tweak, onChange }: IControlProps) => {
  const kind = tweakKind(tweak);
  if (kind === TWEAK_KIND.seg) return <Seg tweak={tweak} onChange={onChange} />;
  if (kind === TWEAK_KIND.colors) return <Dots tweak={tweak} onChange={onChange} />;
  if (kind === TWEAK_KIND.select) return <Dropdown tweak={tweak} onChange={onChange} />;
  if (kind === TWEAK_KIND.step) return <Step tweak={tweak} onChange={onChange} />;
  if (kind === TWEAK_KIND.range) return <Range tweak={tweak} onChange={onChange} />;
  if (kind === TWEAK_KIND.toggle)
    return (
      <button
        type="button"
        className="hx-ds-toggle"
        role="switch"
        aria-checked={tweak.value === true}
        aria-label={tweak.label}
        onClick={() => onChange(tweak.value !== true)}
      >
        <i />
      </button>
    );
  return <Field tweak={tweak} onChange={onChange} numeric={kind === TWEAK_KIND.number} />;
};

export interface ISettingsTabProps {
  /** The settings the board offers now; null while the page has not said them. */
  controls: readonly ITweak[] | null;
  onChange: (name: string, value: TTweakValue) => void;
  /** Puts every setting back as the board is drawn. */
  onReset: () => void;
}

const SettingsTab = ({ controls, onChange, onReset }: ISettingsTabProps) => {
  if (controls === null || controls.length === 0)
    return <div className="hx-ds__empty">{t('next.ds.settings.none')}</div>;
  const groups = tweakGroups(controls);
  const changed = controls.some(tweakChanged);
  return (
    <div className="hx-ds__pane">
      {groups.map((group) => (
        <Fragment key={group.section}>
          {groups.length > 1 && group.section !== '' && <div className="hx-ds-group">{group.section}</div>}
          {group.items.map((tweak) => (
            <div key={tweak.name} className="hx-ds-row" data-kind={tweakKind(tweak)}>
              <span className="hx-ds-row__label">
                {tweak.label}
                {tweakChanged(tweak) && (
                  <b className="hx-ds-changed" aria-label={t('next.ds.settings.changed')} />
                )}
              </span>
              <Control tweak={tweak} onChange={(value) => onChange(tweak.name, value)} />
            </div>
          ))}
        </Fragment>
      ))}
      {changed && (
        <button type="button" className="hx-ds-reset" onClick={onReset}>
          <Icon name="rotate-ccw" size="sm" />
          {t('next.ds.settings.reset')}
        </button>
      )}
    </div>
  );
};

export default SettingsTab;
