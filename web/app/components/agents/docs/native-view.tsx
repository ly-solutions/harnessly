import { useCallback, useEffect, useMemo, useState } from 'react';
import { FileWarning, Lock } from 'lucide-react';
import { DocThreadsError, docAssetSrc, fetchDocNative } from '@/lib/doc-threads-client';
import type { IDocNative, IDocNativeBlock, IDocSceneObject } from '@/types/doc-threads';
import MdPreview from '@/next/features/pane/editor/md-preview';
import { t } from '@/lib/i18n';

// The object as the runtime read it from the provider with the person's own connection, drawn without a frame: used
// when the provider's viewer cannot be embedded. Everything arrives as plain data (the runtime bounds and filters it);
// text is only ever drawn as text, and Markdown never renders raw HTML.

interface INativeViewProps {
  slug: string;
  title: string;
  /** The version of a document to read; null is the current one. */
  version: string | null;
  isBoard: boolean;
}

const Block = ({ block }: { block: IDocNativeBlock }) => {
  switch (block.type) {
    case 'heading': {
      const Tag = `h${Math.min(Math.max(block.level ?? 1, 1), 6)}` as 'h1';
      return <Tag>{block.text}</Tag>;
    }
    case 'quote':
      return <blockquote>{block.text}</blockquote>;
    case 'code':
      return (
        <pre className="hx-md__pre">
          <code>{block.text}</code>
        </pre>
      );
    case 'list':
      return (
        <ul>
          {(block.items ?? []).map((item, i) => (
            <li key={i}>{item}</li>
          ))}
        </ul>
      );
    case 'rule':
      return <hr />;
    default:
      return <p>{block.text}</p>;
  }
};

const center = (o: IDocSceneObject) => ({ x: o.x + (o.w ?? 0) / 2, y: o.y + (o.h ?? 0) / 2 });

/** A board image read through the authorized asset route; one that cannot be read leaves a dashed placeholder. */
const SceneImage = ({ slug, asset, w, h }: { slug: string; asset: string; w: number; h: number }) => {
  const [failed, setFailed] = useState(false);
  if (failed)
    return (
      <rect
        width={w}
        height={h}
        fill="none"
        stroke="currentColor"
        strokeDasharray="4 3"
        data-testid="native-image-missing"
      />
    );
  return (
    <image
      href={docAssetSrc(slug, asset)}
      width={w}
      height={h}
      preserveAspectRatio="xMidYMid meet"
      data-testid="native-image"
      onError={() => setFailed(true)}
    />
  );
};

const Scene = ({ slug, objects, assets }: { slug: string; objects: IDocSceneObject[]; assets: boolean }) => {
  const byId = useMemo(() => new Map(objects.map((o) => [o.id, o])), [objects]);
  const view = useMemo(() => {
    let x0 = Infinity,
      y0 = Infinity,
      x1 = -Infinity,
      y1 = -Infinity;
    for (const o of objects) {
      const pts = [{ x: o.x, y: o.y }, { x: o.x + (o.w ?? 0), y: o.y + (o.h ?? 0) }, ...(o.points ?? [])];
      for (const p of pts) {
        x0 = Math.min(x0, p.x);
        y0 = Math.min(y0, p.y);
        x1 = Math.max(x1, p.x);
        y1 = Math.max(y1, p.y);
      }
    }
    if (!Number.isFinite(x0)) return '0 0 100 100';
    const pad = 40;
    return `${x0 - pad} ${y0 - pad} ${x1 - x0 + 2 * pad} ${y1 - y0 + 2 * pad}`;
  }, [objects]);
  return (
    <svg className="td-native-scene" viewBox={view} role="img" aria-label={t('docs.nativeView.board')}>
      {objects
        .filter((o) => o.type === 'connector')
        .map((o) => {
          const a = o.from ? byId.get(o.from) : undefined;
          const b = o.to ? byId.get(o.to) : undefined;
          const pts = o.points?.length ? o.points : a && b ? [center(a), center(b)] : [];
          if (pts.length < 2) return null;
          return (
            <polyline
              key={o.id}
              data-object-id={o.id}
              data-object-type="connector"
              data-from={o.from}
              data-to={o.to}
              points={pts.map((p) => `${p.x},${p.y}`).join(' ')}
              fill="none"
              stroke="currentColor"
              strokeWidth={2}
            />
          );
        })}
      {objects
        .filter((o) => o.type !== 'connector')
        .map((o) => {
          const w = o.w || 120;
          const h = o.h || 40;
          return (
            <g
              key={o.id}
              data-object-id={o.id}
              data-object-type={o.type}
              data-x={o.x}
              data-y={o.y}
              transform={`translate(${o.x} ${o.y})`}
            >
              {o.type === 'image' ? (
                assets && o.asset ? (
                  <SceneImage slug={slug} asset={o.asset} w={w} h={h} />
                ) : (
                  <rect width={w} height={h} fill="none" stroke="currentColor" strokeDasharray="4 3" />
                )
              ) : (
                <rect
                  width={w}
                  height={h}
                  rx={o.type === 'sticky' ? 2 : 6}
                  className={o.type === 'sticky' ? 'td-native-sticky' : 'td-native-shape'}
                />
              )}
              {o.text && (
                <foreignObject width={w} height={h}>
                  <div className="td-native-text">{o.text}</div>
                </foreignObject>
              )}
            </g>
          );
        })}
    </svg>
  );
};

/** What a person is told about an object that could not be read natively: the object, not the catalog. */
const failureOf = (error: unknown): { denied: boolean; text: string } => {
  if (error instanceof DocThreadsError && (error.code === 'denied' || error.status === 403))
    return { denied: true, text: error.message };
  return { denied: false, text: error instanceof Error ? error.message : t('docs.nativeView.failed') };
};

const NativeView = ({ slug, title, version, isBoard }: INativeViewProps) => {
  const [first, setFirst] = useState<IDocNative | null>(null);
  const [objects, setObjects] = useState<IDocSceneObject[]>([]);
  const [next, setNext] = useState<string | null>(null);
  const [error, setError] = useState<{ denied: boolean; text: string } | null>(null);
  const [more, setMore] = useState(false);

  useEffect(() => {
    let live = true;
    // eslint-disable-next-line react-hooks/set-state-in-effect -- a new object starts from an empty pane
    setFirst(null);
    setObjects([]);
    setNext(null);
    setError(null);
    fetchDocNative(slug, { version })
      .then((n) => {
        if (!live) return;
        setFirst(n);
        setObjects(n.scene?.objects ?? []);
        setNext(n.scene?.next || null);
      })
      .catch((e: unknown) => live && setError(failureOf(e)));
    return () => {
      live = false;
    };
  }, [slug, version]);

  const loadMore = useCallback(() => {
    if (!next) return;
    setMore(true);
    fetchDocNative(slug, { cursor: next })
      .then((n) => {
        setObjects((was) => {
          const seen = new Set(was.map((o) => o.id));
          return [...was, ...(n.scene?.objects ?? []).filter((o) => !seen.has(o.id))];
        });
        setNext(n.scene?.next || null);
      })
      .catch((e: unknown) => setError(failureOf(e)))
      .finally(() => setMore(false));
  }, [slug, next]);

  if (error)
    return (
      <div
        className="td-native td-native-note"
        role="alert"
        data-testid="native-error"
        data-denied={error.denied}
      >
        {error.denied ? <Lock aria-hidden="true" /> : <FileWarning aria-hidden="true" />}
        <p>{error.denied ? `${error.text} (${title})` : error.text}</p>
      </div>
    );
  if (!first)
    return (
      <div className="td-native td-native-note" role="status">
        <p>{t('docs.nativeView.loading')}</p>
      </div>
    );
  if (first.representation === 'unsupported' || (isBoard && first.representation !== 'board'))
    return (
      <div className="td-native td-native-note" data-testid="native-unsupported">
        <FileWarning aria-hidden="true" />
        <p>{t('docs.nativeView.unsupported')}</p>
        {first.reason && <p className="td-native-reason">{first.reason}</p>}
      </div>
    );
  if (first.representation === 'board')
    return (
      <div className="td-native" data-testid="native-board" data-count={objects.length}>
        <Scene slug={slug} objects={objects} assets={Boolean(first.assets)} />
        {next && (
          <button type="button" className="td-native-more" onClick={loadMore} disabled={more}>
            {t('docs.nativeView.more')}
          </button>
        )}
      </div>
    );
  return (
    <div className="td-native" data-testid="native-document">
      {first.representation === 'markdown' ? (
        <MdPreview md={first.markdown ?? ''} />
      ) : (
        <div className="hx-md hx-fed__doc">
          {(first.blocks ?? []).map((block, i) => (
            <Block key={i} block={block} />
          ))}
        </div>
      )}
    </div>
  );
};

export default NativeView;
