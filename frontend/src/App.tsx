import { useState } from 'react';
import DecisionCard, { ForecastBadge } from './DecisionCard';
import EvidencePanel from './EvidencePanel';
import Uncertainty from './Uncertainty';
import { useInsights, useRecommendation } from './hooks';
import { areaName, date } from './presentation';

export default function App() {
  const { result, loading, error, expired, refresh } = useRecommendation();
  const [selection, setSelection] = useState<{ decision: string; id: number } | null>(null);
  const usable = !!result && result.status !== 'unavailable' && !expired;
  // A new decision defaults to its recommended zone. Manual inspection stays
  // selected while that decision is unchanged and never changes its action.
  const decision = `${result?.forecast_version}/${result?.window.start}/${result?.primary_recommendation?.area.id ?? 'none'}/${result?.status}`;
  const manual = selection?.decision === decision && result?.areas.some(item => item.area.id === selection.id);
  const areaID = manual ? selection.id : result?.primary_recommendation?.area.id ?? result?.areas[0]?.area.id ?? null;
  const detail = useInsights(result, areaID, expired, refresh);

  return <main>
    <header className="app-header">
      <div><h1>NYC Operations Control Room</h1><p className="subtitle">Weather-informed mobility operations</p></div>
      <div className="header-actions"><ForecastBadge freshness={expired ? 'unavailable' : result?.freshness ?? 'unavailable'} />
        <button type="button" onClick={() => void refresh()} disabled={loading}>{loading ? 'Refreshing…' : 'Refresh'}</button></div>
    </header>
    <p className="scope-note">This decision concerns weather-related trip activity, not the overall health of marketplace operations.</p>
    {result && <p className="planning-window"><span aria-hidden="true">◷</span> {date(result.window.start)} – {date(result.window.end)} · New York</p>}
    {error && <p className="notice" role="alert">{error}</p>}
    {loading && !result && !error && <p role="status">Loading the current recommendation…</p>}
    {expired && <p className="notice" role="status">The previous decision has expired. Waiting for a current result; no action is shown.</p>}
    <div aria-live="polite">{result && !expired && <DecisionCard result={result} />}</div>

    {usable && <section className="supporting" aria-labelledby="evidence-title">
      <h2 id="evidence-title">Supporting evidence</h2>
      <div className="zone-selection"><label htmlFor="area" className="sr-only">Inspect a monitored zone</label>
        <select id="area" value={areaID ?? ''} onChange={event => setSelection({ decision, id: Number(event.target.value) })}>
          {result.areas.map(item => <option key={item.area.id} value={item.area.id}>{areaName(item.area.name)}</option>)}
        </select><span className="muted">Selected zone</span></div>
      {detail.error ? <p role="alert">{detail.error}</p>
        : detail.insights ? <EvidencePanel evidence={detail.insights.evidence} result={result} />
          : <p role="status">Loading area evidence…</p>}
    </section>}

    {result && !expired && <Uncertainty result={result} />}
    <footer>Automatically refreshed every 60 seconds · NYC TLC + Open-Meteo</footer>
  </main>;
}
