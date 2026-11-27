import { useState } from 'react';
import DecisionCard, { ForecastBadge } from './components/DecisionCard';
import EvidencePanel from './components/EvidencePanel';
import ZoneImpactRanking from './components/ZoneImpactRanking';
import Uncertainty from './components/Uncertainty';
import { useInsights, useRecommendation } from './state/hooks';
import { formatAreaName, formatNewYorkTime, rankAssessedZones } from './utils/presentation';

export default function App() {
    const { result, loading, error, expired, refresh } = useRecommendation();
    const [selection, setSelection] = useState<{ decision: string; id: number } | null>(null);
    const usable = !!result && result.status !== 'unavailable' && !expired;
    // A new decision defaults to its recommended zone. Manual inspection stays
    // selected while that decision is unchanged and never changes its action.
    const decision = `${result?.forecast_version}/${result?.window.start}/${result?.primary_recommendation?.area.id ?? 'none'}/${result?.status}`;

    const manual =
        selection?.decision === decision &&
        result?.areas.some((item) => item.area.id === selection.id);

    const ranked = rankAssessedZones(result?.areas ?? []);

    const areaID = manual
        ? selection.id
        : (result?.primary_recommendation?.area.id ??
          ranked[0]?.area.id ??
          result?.areas[0]?.area.id ??
          null);

    const selectedArea = result?.areas.find((item) => item.area.id === areaID)?.area;
    const detail = useInsights(result, areaID, expired, refresh);

    return (
        <main>
            <header className="app-header">
                <div>
                    <h1>NYC Operations Control Room</h1>
                    <p className="subtitle">Weather-informed mobility operations</p>
                </div>
                <div className="header-actions">
                    <ForecastBadge
                        freshness={expired ? 'unavailable' : (result?.freshness ?? 'unavailable')}
                    />
                    <button type="button" onClick={() => void refresh()} disabled={loading}>
                        {loading ? 'Refreshing…' : 'Refresh'}
                    </button>
                </div>
            </header>
            <p className="scope-note">
                This decision concerns weather-related trip activity, not the overall health of
                marketplace operations.
            </p>
            {result && (
                <p className="planning-window">
                    <span aria-hidden="true">◷</span> {formatNewYorkTime(result.window.start)} –{' '}
                    {formatNewYorkTime(result.window.end)} · New York
                </p>
            )}
            {error && (
                <p className="notice" role="alert">
                    {error}
                </p>
            )}
            {loading && !result && !error && (
                <p role="status">Loading the current recommendation…</p>
            )}
            {expired && (
                <p className="notice" role="status">
                    The previous decision has expired. Waiting for a current result; no action is
                    shown.
                </p>
            )}
            <div aria-live="polite">{result && !expired && <DecisionCard result={result} />}</div>

            {usable && (
                <ZoneImpactRanking
                    key={decision}
                    result={result}
                    ranked={ranked}
                    selectedID={areaID}
                    onSelect={(id) => setSelection({ decision, id })}
                />
            )}
            {usable && selectedArea && (
                <section id="zone-evidence" className="supporting" aria-labelledby="evidence-title">
                    <h2 id="evidence-title">
                        Supporting evidence · {formatAreaName(selectedArea.name)}
                    </h2>
                    {detail.error ? (
                        <p role="alert">{detail.error}</p>
                    ) : detail.insights ? (
                        <EvidencePanel evidence={detail.insights.evidence} result={result} />
                    ) : (
                        <p role="status">Loading area evidence…</p>
                    )}
                </section>
            )}

            {result && !expired && <Uncertainty result={result} />}
            <footer>Automatically refreshed every 60 seconds · NYC TLC + Open-Meteo</footer>
        </main>
    );
}
