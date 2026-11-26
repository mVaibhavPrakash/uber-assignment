import { number, signedNumber } from "../utils/presentation";
import type { AreaEvidence } from "../types/recommendation";

// Both bars share one scale. Labels keep the comparison readable without color.
export default function ComparisonBars({
  evidence,
}: {
  evidence: AreaEvidence;
}) {
  const baseline = evidence.baseline_trips;
  const estimated = evidence.estimated_trips;
  if (!evidence.assessable || baseline == null || estimated == null) {
    return (
      <p className="muted">
        A full-window trip comparison is unavailable because this zone has
        incomplete evidence.
      </p>
    );
  }
  const maximum = Math.max(baseline, estimated);
  const width = (value: number) =>
    maximum === 0 ? "0%" : `${(value / maximum) * 100}%`;
  return (
    <div className="comparison">
      <div className="bar-row">
        <div className="bar-label">
          <span>Typical dry-weather trips</span>
          <strong>{number(baseline)}</strong>
        </div>
        <div className="bar-track">
          <div className="bar baseline" style={{ width: width(baseline) }} />
        </div>
      </div>
      <div className="bar-row">
        <div className="bar-label">
          <span>Expected trips with current forecast</span>
          <strong>{number(estimated)}</strong>
        </div>
        <div className="bar-track">
          <div className="bar estimated" style={{ width: width(estimated) }} />
        </div>
      </div>
      <p className="muted comparison-caption">
        {evidence.additional_trips === 0
          ? "No additional rain-related trip volume is projected."
          : `${signedNumber(evidence.additional_trips)} completed trips compared with the dry-weather baseline.`}
      </p>
    </div>
  );
}
