import type { ReactNode } from "react";
import ComparisonBars from "./ComparisonBars";
import { formatAreaName, formatNewYorkTime, formatPlanningDuration, formatNumber, formatSignedNumber } from "../utils/presentation";
import type { Recommendation } from "../types/recommendation";

export function ForecastBadge({ freshness }: { freshness: string }) {
  const label =
    freshness === "fresh"
      ? "Forecast fresh"
      : freshness === "stale"
        ? "Forecast stale"
        : "Forecast unavailable";
  return (
    <span className={`badge ${freshness}`}>
      <span aria-hidden="true">●</span> {label}
    </span>
  );
}

export default function DecisionCard({ result }: { result: Recommendation }) {
  const action = result.status === "action_required";
  const primary = action ? result.primary_recommendation : null;
  const normal = result.status === "no_intervention";
  const partial = result.coverage.assessed < result.coverage.total;
  const title = primary
    ? `Review driver availability in ${formatAreaName(primary.area.name)}`
    : normal
      ? partial ? "No assessed zone reaches the intervention threshold" : "Continue normal operations"
      : result.status === "insufficient_evidence"
        ? "Insufficient evidence to recommend an action"
        : "Recommendation unavailable";
  const instruction = primary
    ? `Review driver availability in ${primary.area.name}. `
    : "";
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
          {normal ? "✓" : action ? "!" : "?"}
        </span>
        <div>
          <p className="eyebrow">
            {normal || action ? "Recommended action" : "Decision status"}
          </p>
          <h2 id="decision-title">{title}</h2>
        </div>
      </div>
      <p className="decision-explanation">{explanation}</p>
      {partial && (
        <p className="notice" role="status">
          {result.coverage.assessed} of {result.coverage.total} zones assessed. {result.coverage.total - result.coverage.assessed} zones could not be assessed
          because forecast or historical evidence is incomplete or weather is unsupported.
          {result.coverage.assessed > 0 && " A higher-impact zone may be among them. This decision covers assessed zones only."}
        </p>
      )}
      {result.freshness === "stale" && (
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
              Highest eligible additional-trip estimate across{" "}
              {result.coverage.assessed} assessed zones out of {result.coverage.total} monitored. Intervention threshold:{" "}
              {formatNumber(result.threshold_additional_trips)} trips.
            </p>
          </div>
          <div className="before-acting">
            <h3>Before acting</h3>
            <p>
              Check current driver availability. Historical completed trips
              suggest a possible increase in activity; they do not establish a
              driver shortage.
            </p>
          </div>
        </>
      )}
      <p className="decision-footer muted">
        {result.forecast_fetched_at ? (
          <>
            Forecast fetched at {formatNewYorkTime(result.forecast_fetched_at, false)} ·
            Decision valid until {formatNewYorkTime(result.valid_until, false)}
          </>
        ) : (
          "Waiting for a current, complete forecast and recommendation."
        )}
      </p>
    </section>
  );
}

// Select metric values separately from their shared presentation.
function DecisionMetrics({ result }: { result: Recommendation }) {
  const primary = result.status === "action_required" ? result.primary_recommendation : null;
  const zones = `${result.coverage.assessed}/${result.coverage.total}`;
  const trips = (value: number | null) => <>{formatNumber(value)} <span>trips</span></>;
  let metrics: [string, ReactNode][];
  if (primary) {
    metrics = [
      ["Expected additional trips", formatSignedNumber(primary.additional_trips)],
      ["Expected increase", primary.relative_uplift_percent == null
        ? "Not available" : `${formatNumber(primary.relative_uplift_percent)}%`],
      ["Planning window", formatPlanningDuration(result)],
    ];
  } else if (result.status === "no_intervention") {
    metrics = [
      ["Zones assessed", zones],
      [(result.highest_modeled_additional_trips ?? 0) < 0
        ? "Largest expected change" : "Largest expected increase",
        trips(result.highest_modeled_additional_trips)],
      ["Intervention threshold", trips(result.threshold_additional_trips)],
    ];
  } else {
    metrics = [
      ["Zones assessed", zones],
      ["Expected impact", "Not available"],
      ["Planning window", formatPlanningDuration(result)],
    ];
  }
  return <dl className="metrics">{metrics.map(([label, value]) => (
    <div key={label}><dt>{label}</dt><dd>{value}</dd></div>
  ))}</dl>;
}
