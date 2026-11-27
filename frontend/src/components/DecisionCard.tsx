import type { ReactNode } from 'react';
import ComparisonBars from './ComparisonBars';
import {
    formatAreaName,
    formatNewYorkTime,
    formatPlanningDuration,
    formatNumber,
    formatSignedNumber,
} from '../utils/presentation';
import type { Recommendation } from '../types/recommendation';

export function ForecastBadge({ freshness }: { freshness: string }) {
    const label =
        freshness === 'fresh' || freshness === 'stale'
            ? `Forecast ${freshness}`
            : 'Forecast unavailable';

    return (
        <span className={`badge ${freshness}`}>
            <span aria-hidden="true">●</span> {label}
        </span>
    );
}

function decisionTitle(result: Recommendation): string {
    switch (result.status) {
        case 'action_required':
            return result.primary_recommendation
                ? `Review driver availability in ${formatAreaName(result.primary_recommendation.area.name)}`
                : 'Recommendation unavailable';
        case 'no_intervention':
            return result.decision_reason === 'no_significant_rainfall'
                ? 'No rain-related action needed'
                : 'No zone reaches the intervention threshold';
        case 'insufficient_evidence':
            return 'Insufficient evidence to recommend an action';
        default:
            return 'Recommendation unavailable';
    }
}

export default function DecisionCard({ result }: { result: Recommendation }) {
    const action = result.status === 'action_required';
    const primary = action ? result.primary_recommendation : null;
    const normal = result.status === 'no_intervention';
    const partial = result.coverage.assessed < result.coverage.total;
    const available = result.status !== 'unavailable';
    const fresh = available && result.freshness === 'fresh';
    const instruction = primary ? `Review driver availability in ${primary.area.name}. ` : '';

    const timing = [
        available &&
            result.forecast_fetched_at &&
            `Forecast retrieved: ${formatNewYorkTime(result.forecast_fetched_at)}`,
        fresh && `Next decision update: ${formatNewYorkTime(result.window.start)}`,
        available &&
            (!fresh || Date.parse(result.valid_until) < Date.parse(result.window.start)) &&
            `Decision expires: ${formatNewYorkTime(result.valid_until)}`,
    ]
        .filter(Boolean)
        .join(' · ');

    const explanation =
        instruction && result.message.startsWith(instruction)
            ? result.message.slice(instruction.length)
            : result.message;

    return (
        <section
            className={`decision ${result.status}`}
            data-status={result.status}
            aria-labelledby="decision-title"
        >
            <div className="decision-heading">
                <span className="status-icon" aria-hidden="true">
                    {normal ? '✓' : action ? '!' : '?'}
                </span>
                <div>
                    <p className="eyebrow">
                        {normal || action ? 'Recommended action' : 'Decision status'}
                    </p>
                    <h2 id="decision-title">{decisionTitle(result)}</h2>
                </div>
            </div>
            <p className="decision-explanation">{explanation}</p>
            {partial && (
                <p className="notice" role="status">
                    {result.coverage.assessed} of {result.coverage.total} zones assessed.{' '}
                    {result.coverage.total - result.coverage.assessed} zones could not be assessed
                    because forecast or historical evidence is incomplete or weather is unsupported.
                    {result.coverage.assessed > 0 &&
                        ' A higher-impact zone may be among them. This decision covers assessed zones only.'}
                </p>
            )}
            {result.freshness === 'stale' && (
                <p className="notice" role="status">
                    Supporting weather is stale. Check current conditions before acting.
                </p>
            )}
            <DecisionMetrics result={result} />
            {primary && (
                <>
                    <div className="decision-evidence">
                        <h3>Why this zone?</h3>
                        <ComparisonBars evidence={primary} />
                        <p className="muted">
                            Highest estimated additional trips among {result.coverage.assessed}{' '}
                            assessed zones out of {result.coverage.total} monitored. Intervention
                            threshold: {formatNumber(result.threshold_additional_trips)} trips.
                        </p>
                    </div>
                    <div className="before-acting">
                        <h3>Before acting</h3>
                        <p>
                            Check current driver availability. Historical completed trips suggest a
                            possible increase in activity; they do not establish a driver shortage.
                        </p>
                    </div>
                </>
            )}
            <p className="decision-footer muted">
                {timing || 'Waiting for a current, complete forecast and recommendation.'}
            </p>
        </section>
    );
}

// Select metric values separately from their shared presentation.
function DecisionMetrics({ result }: { result: Recommendation }) {
    const primary = result.status === 'action_required' ? result.primary_recommendation : null;
    const zones = `${result.coverage.assessed}/${result.coverage.total}`;

    const trips = (value: number | null) => (
        <>
            {formatNumber(value)} <span>trips</span>
        </>
    );

    let metrics: [string, ReactNode][];

    if (primary) {
        metrics = [
            ['Estimated additional trips', formatSignedNumber(primary.additional_trips)],
            [
                'Estimated increase',
                primary.relative_uplift_percent == null
                    ? 'Not available'
                    : `${formatNumber(primary.relative_uplift_percent)}%`,
            ],
            ['Planning window', formatPlanningDuration(result)],
        ];
    } else if (result.status === 'no_intervention') {
        metrics = [
            ['Zones assessed', zones],
            [
                (result.highest_modeled_additional_trips ?? 0) < 0
                    ? 'Largest estimated change'
                    : 'Highest estimated additional trips',
                trips(result.highest_modeled_additional_trips),
            ],
            ['Intervention threshold', trips(result.threshold_additional_trips)],
        ];
    } else {
        metrics = [
            ['Zones assessed', zones],
            ['Estimated additional trips', 'Not available'],
            ['Planning window', formatPlanningDuration(result)],
        ];
    }

    return (
        <dl className="metrics">
            {metrics.map(([label, value]) => (
                <div key={label}>
                    <dt>{label}</dt>
                    <dd>{value}</dd>
                </div>
            ))}
        </dl>
    );
}
