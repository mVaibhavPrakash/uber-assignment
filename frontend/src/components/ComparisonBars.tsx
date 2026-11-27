import { formatNumber, formatSignedNumber } from '../utils/presentation';
import type { AreaEvidence } from '../types/recommendation';

// Both bars share one scale. Labels keep the comparison readable without color.
export default function ComparisonBars({ evidence }: { evidence: AreaEvidence }) {
    const baseline = evidence.baseline_trips;
    const estimated = evidence.estimated_trips;
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
                Typical and expected trip totals are unavailable for the full window.{' '}
                {evidence.additional_trips === 0
                    ? 'No additional rain-related trips are projected.'
                    : `Modeled rain-related change: ${formatSignedNumber(evidence.additional_trips)} trips.`}
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
                    ['estimated', 'Expected trips with current forecast', estimated],
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
                {evidence.additional_trips === 0
                    ? 'No additional rain-related trip volume is projected.'
                    : `${formatSignedNumber(evidence.additional_trips)} completed trips compared with typical dry-weather trips.`}
            </p>
        </div>
    );
}
