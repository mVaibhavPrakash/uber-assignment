import { useState } from 'react';
import type { AreaEvidence, Recommendation } from '../types/recommendation';
import { type AssessedZone, formatAreaName, formatSignedNumber } from '../utils/presentation';

type Props = {
    result: Recommendation;
    ranked: AssessedZone[];
    selectedID: number | null;
    onSelect: (id: number) => void;
};

const previewSize = 5;

export default function ZoneImpactRanking({ result, ranked, selectedID, onSelect }: Props) {
    const [expanded, setExpanded] = useState(false);
    const unknown = result.areas.filter((summary) => !summary.assessable);
    const visibleRanked = expanded ? ranked : ranked.slice(0, previewSize);
    const visibleUnknown = expanded ? unknown : unknown.slice(0, ranked.length ? 0 : previewSize);
    const previewCount = Math.min(previewSize, ranked.length || unknown.length);
    const threshold = result.threshold_additional_trips_exact;
    const qualifying = ranked.filter((zone) => zone.additional_trips_exact >= threshold).length;

    function zoneRow(summary: AreaEvidence, status: string) {
        return (
            <li key={summary.area.id}>
                <button
                    type="button"
                    className="zone-row"
                    aria-pressed={summary.area.id === selectedID}
                    aria-controls="zone-evidence"
                    onClick={() => onSelect(summary.area.id)}
                >
                    <span className="zone-name">{formatAreaName(summary.area.name)}</span>
                    <strong className="zone-impact">
                        <span className="sr-only">Expected extra trips: </span>
                        {summary.assessable
                            ? formatSignedNumber(summary.additional_trips)
                            : 'Unknown'}
                    </strong>
                    <span className="zone-status">{status}</span>
                </button>
            </li>
        );
    }

    return (
        <section className="supporting ranking" aria-labelledby="ranking-title">
            <h2 id="ranking-title">Zone impact ranking</h2>
            <p className="muted">
                {ranked.length === 0
                    ? 'No zones could be assessed for this window.'
                    : `${qualifying} of ${ranked.length} assessed at or above the intervention threshold (${threshold.toLocaleString('en-US', { maximumSignificantDigits: 15 })} extra trips).`}
                {ranked.length > 0 &&
                    unknown.length > 0 &&
                    ` Insufficient evidence for ${unknown.length} of ${result.areas.length} zones.`}
            </p>

            <div id="zone-list">
                {ranked.length > 0 && (
                    <>
                        <div className="zone-columns muted" aria-hidden="true">
                            <span>Zone</span>
                            <span>Expected extra trips</span>
                            <span>Status</span>
                        </div>
                        <ol
                            className="zone-list"
                            aria-label="Assessed zones by expected extra trips"
                        >
                            {visibleRanked.map((summary, index) => {
                                const primary =
                                    summary.area.id === result.primary_recommendation?.area.id;

                                const status = primary
                                    ? 'Primary recommendation'
                                    : index < qualifying
                                      ? 'Meets threshold'
                                      : 'Below threshold';

                                return zoneRow(summary, status);
                            })}
                        </ol>
                    </>
                )}
                {visibleUnknown.length > 0 && (
                    <>
                        <h3>Insufficient evidence</h3>
                        <ul className="zone-list" aria-label="Zones with unknown impact">
                            {visibleUnknown.map((summary) => zoneRow(summary, ''))}
                        </ul>
                    </>
                )}
            </div>

            {result.areas.length > previewCount && (
                <button
                    type="button"
                    className="zone-toggle"
                    aria-expanded={expanded}
                    aria-controls="zone-list"
                    onClick={() => setExpanded(!expanded)}
                >
                    {expanded ? 'Show fewer zones' : `View all zones (${result.areas.length})`}
                </button>
            )}
        </section>
    );
}
