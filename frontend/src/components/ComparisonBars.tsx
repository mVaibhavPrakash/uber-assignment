import { formatNumber, formatSignedNumber } from '../utils/presentation';
import type { AreaEvidence } from '../types/recommendation';

// Both bars share one scale. Labels keep the comparison readable without color.
export default function ComparisonBars({ evidence }: { evidence: AreaEvidence }) {
    const baseline = evidence.baseline_trips;
    const estimated = evidence.estimated_trips;
    const additional = formatSignedNumber(evidence.additional_trips);
    if (!evidence.assessable) {
        return (
            <p className="muted">
                A full-window trip comparison is unavailable because this zone has incomplete
                evidence.
            </p>
        );
    }

    if (baseline == null || estimated == null) {
        return (
            <p className="muted">
                Typical and estimated trip totals are unavailable for the full window. Estimated
                additional trips: {additional}.
            </p>
        );
    }

    const maximum = Math.max(baseline, estimated);
    const width = (value: number) => (maximum === 0 ? '0%' : `${(value / maximum) * 100}%`);

    return (
        <div className="comparison">
            {(
                [
                    ['baseline', 'Typical dry-weather trips', baseline],
                    ['estimated', 'Estimated trips with current forecast', estimated],
                ] as const
            ).map(([kind, label, value]) => (
                <div className="bar-row" key={kind}>
                    <div className="bar-label">
                        <span
                            title={
                                kind === 'baseline'
                                    ? 'Includes dry conditions and light rain below the configured rain threshold.'
                                    : undefined
                            }
                        >
                            {label}
                        </span>
                        <strong>{formatNumber(value)}</strong>
                    </div>
                    <div className="bar-track">
                        <div className={`bar ${kind}`} style={{ width: width(value) }} />
                    </div>
                </div>
            ))}

            <p className="muted comparison-caption">
                Estimated additional trips: {additional} compared with typical dry-weather trips.
            </p>
        </div>
    );
}
