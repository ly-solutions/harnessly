// The variables a provider adds to the launch of Claude Code on it: the base ones typed by hand and any
// others. An empty field leaves the variable as the launch makes it; a filled one always replaces it. A secret value
// is typed and never shown again: the machine keeps it in its safe and says only that it is set.
import type { IExtEnvInput, IExtEnvVar } from '@/next/features/settings/ext-providers-api';
import { SetButton } from '@/next/features/settings/rows';
import { t } from '@/next/i18n';
import Icon from '@/next/ui/icon';
import SecretField from '@/next/ui/secret-field';

/** The base variables of Claude Code, in the order of the form; the secret ones are kept in the safe. */
const BASE = [
  { name: 'ANTHROPIC_BASE_URL', secret: false },
  { name: 'ANTHROPIC_MODEL', secret: false },
  { name: 'ANTHROPIC_SMALL_FAST_MODEL', secret: false },
  { name: 'ANTHROPIC_AUTH_TOKEN', secret: true },
  { name: 'ANTHROPIC_API_KEY', secret: true },
  { name: 'NODE_EXTRA_CA_CERTS', secret: false },
  { name: 'SSL_CERT_FILE', secret: false },
] as const;

const BASE_NAMES: readonly string[] = BASE.map((field) => field.name);

/** The machine keeps a variable of such a name in the safe whatever the form says (extproviders.EnvAlwaysSecret). */
const SECRET_NAME = /(KEY|TOKEN|SECRET|PASSWORD|PASSWD|CREDENTIALS?)$/i;

export interface IEnvRow {
  /** Stable across edits of the name. */
  id: string;
  name: string;
  /** What is typed now; '' for a secret that is kept and not typed again. */
  value: string;
  secret: boolean;
  /** The safe has the value of this secret variable. */
  kept: boolean;
}

/** The form's rows of what the machine keeps. */
export const rowsOf = (env: readonly IExtEnvVar[]): IEnvRow[] =>
  env.map((item, i) => ({
    id: `kept-${i}`,
    name: item.name,
    value: item.secret ? '' : item.value,
    secret: item.secret,
    kept: item.secret && item.set,
  }));

/** The rows as the machine takes them: an empty plain variable and a secret with nothing typed or kept are left out. */
export const inputsOf = (rows: readonly IEnvRow[]): IExtEnvInput[] =>
  rows.flatMap((row): IExtEnvInput[] => {
    const name = row.name.trim();
    const value = row.value.trim();
    if (name === '') return [];
    const secret = row.secret || SECRET_NAME.test(name);
    if (value !== '') return [{ name, secret, value }];
    return secret && row.kept ? [{ name, secret }] : [];
  });

/** Whether the rows say something else than the machine keeps. */
export const envDirty = (rows: readonly IEnvRow[], env: readonly IExtEnvVar[]): boolean =>
  JSON.stringify(inputsOf(rows)) !== JSON.stringify(inputsOf(rowsOf(env)));

interface IEnvEditorProps {
  rows: readonly IEnvRow[];
  disabled: boolean;
  onChange: (rows: IEnvRow[]) => void;
}

const EnvEditor = ({ rows, disabled, onChange }: IEnvEditorProps) => {
  const baseRow = (name: string) => rows.find((row) => row.name === name);
  const setBase = (name: string, secret: boolean, value: string) => {
    const have = baseRow(name);
    onChange(
      have
        ? rows.map((row) => (row === have ? { ...row, value, kept: row.kept && value === '' } : row))
        : [...rows, { id: `base-${name}`, name, value, secret, kept: false }],
    );
  };
  const extras = rows.filter((row) => !BASE_NAMES.includes(row.name));
  const patch = (id: string, change: Partial<IEnvRow>) =>
    onChange(rows.map((row) => (row.id === id ? { ...row, ...change } : row)));
  const keptHint = (row: IEnvRow | undefined) =>
    row?.kept ? t('next.extProviders.env.kept') : t('next.extProviders.env.default');

  return (
    <div className="hx-extp__fld hx-extp__env">
      <span className="hx-extp__label">{t('next.extProviders.env.title')}</span>
      <small className="hx-extp__hint">{t('next.extProviders.env.hint')}</small>
      {BASE.map((field) => {
        const row = baseRow(field.name);
        return (
          <div key={field.name} className="hx-extp__var">
            <span className="hx-extp__var-name">{field.name}</span>
            {field.secret ? (
              <SecretField
                value={row?.value ?? ''}
                onChange={(value) => setBase(field.name, true, value)}
                label={field.name}
                placeholder={keptHint(row)}
                disabled={disabled}
              />
            ) : (
              <input
                className="hx-set__input hx-set__input--mono"
                value={row?.value ?? ''}
                disabled={disabled}
                autoCapitalize="off"
                spellCheck={false}
                aria-label={field.name}
                placeholder={t('next.extProviders.env.default')}
                onChange={(event) => setBase(field.name, false, event.target.value)}
              />
            )}
            {row?.kept && (
              <button
                type="button"
                className="hx-head-btn"
                aria-label={t('next.extProviders.env.clear', { name: field.name })}
                disabled={disabled}
                onClick={() => onChange(rows.filter((item) => item !== row))}
              >
                <Icon name="x" size="sm" />
              </button>
            )}
          </div>
        );
      })}
      {extras.map((row) => {
        const secret = row.secret || SECRET_NAME.test(row.name);
        return (
          <div key={row.id} className="hx-extp__var" data-own="true">
            <input
              className="hx-set__input hx-set__input--mono hx-extp__var-key"
              value={row.name}
              disabled={disabled}
              maxLength={64}
              autoCapitalize="off"
              spellCheck={false}
              aria-label={t('next.extProviders.env.name')}
              placeholder={t('next.extProviders.env.name')}
              onChange={(event) => patch(row.id, { name: event.target.value })}
            />
            {secret ? (
              <SecretField
                value={row.value}
                onChange={(value) => patch(row.id, { value, kept: row.kept && value === '' })}
                label={t('next.extProviders.env.value')}
                placeholder={row.kept ? t('next.extProviders.env.kept') : t('next.extProviders.env.value')}
                disabled={disabled}
              />
            ) : (
              <input
                className="hx-set__input hx-set__input--mono"
                value={row.value}
                disabled={disabled}
                autoCapitalize="off"
                spellCheck={false}
                aria-label={t('next.extProviders.env.value')}
                placeholder={t('next.extProviders.env.value')}
                onChange={(event) => patch(row.id, { value: event.target.value })}
              />
            )}
            <button
              type="button"
              className="hx-head-btn"
              aria-label={t('next.extProviders.env.secret')}
              aria-pressed={secret}
              disabled={disabled || SECRET_NAME.test(row.name)}
              onClick={() => patch(row.id, { secret: !row.secret, value: '', kept: false })}
            >
              <Icon name={secret ? 'lock' : 'lock-open'} size="sm" />
            </button>
            <button
              type="button"
              className="hx-head-btn"
              aria-label={t('next.extProviders.env.remove')}
              disabled={disabled}
              onClick={() => onChange(rows.filter((item) => item.id !== row.id))}
            >
              <Icon name="x" size="sm" />
            </button>
          </div>
        );
      })}
      <SetButton
        tone="ghost"
        icon="plus"
        disabled={disabled}
        onClick={() =>
          onChange([...rows, { id: `own-${Date.now()}`, name: '', value: '', secret: false, kept: false }])
        }
      >
        {t('next.extProviders.env.add')}
      </SetButton>
    </div>
  );
};

export default EnvEditor;
