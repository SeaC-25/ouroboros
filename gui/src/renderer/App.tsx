import { lazy, Suspense, useEffect } from 'react';
import { activateTheme } from '../app/theme';
import '../styles/ouroboros-ide.css';

// The IDE is a large synchronous surface (25k-line workbench); loading it
// lazily splits the bundle so the shell paints instantly with a skeleton
// while the workbench chunk streams in.
const OuroborosIDE = lazy(() =>
  import('../app/OuroborosIDE').then((m) => ({ default: m.OuroborosIDE })),
);

function IDESkeleton() {
  return (
    <div className="ide-skeleton" aria-busy="true" aria-label="正在加载工作台">
      <div className="ide-skeleton__topbar">
        <span className="ide-skeleton__logo" />
        <span className="ide-skeleton__tab" />
        <span className="ide-skeleton__tab ide-skeleton__tab--short" />
      </div>
      <div className="ide-skeleton__body">
        <aside className="ide-skeleton__side">
          <span className="ide-skeleton__row" />
          <span className="ide-skeleton__row" />
          <span className="ide-skeleton__row ide-skeleton__row--short" />
          <span className="ide-skeleton__row" />
        </aside>
        <main className="ide-skeleton__main">
          <span className="ide-skeleton__row" />
          <span className="ide-skeleton__row ide-skeleton__row--wide" />
          <span className="ide-skeleton__row" />
        </main>
      </div>
      <div className="ide-skeleton__statusbar">
        <span className="ide-skeleton__pulse">正在加载 Ouroboros 工作台…</span>
      </div>
    </div>
  );
}

export default function App() {
  useEffect(() => activateTheme(), []);
  return (
    <Suspense fallback={<IDESkeleton />}>
      <OuroborosIDE />
    </Suspense>
  );
}
