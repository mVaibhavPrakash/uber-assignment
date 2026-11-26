import { Fragment } from 'react';
import ComparisonBars from './ComparisonBars';
import { formatAreaName, describeHourAssessment, formatNewYorkTime, formatPlanningDuration, formatNumber } from '../utils/presentation';
import type { AreaEvidence, Recommendation } from '../types/recommendation';

export default function EvidencePanel({ evidence, result }: { evidence: AreaEvidence; result: Recommendation }) {
  return <>
    <div className="evidence-card"><h3>Expected trips over the next {formatPlanningDuration(result)}</h3>
      <ComparisonBars evidence={evidence} /></div>
    <h3>Hourly breakdown</h3>
    <div className="table-scroll" tabIndex={0} role="region" aria-label={`${formatAreaName(evidence.area.name)} hourly evidence`}>
      <table className="hourly-table">
        <thead><tr><th scope="col">Time</th><th scope="col">Rain forecast</th>
          <th scope="col">Typical trips</th><th scope="col">Extra trips</th><th scope="col">Evidence</th></tr></thead>
        <tbody>{evidence.hourly?.map(hour => <Fragment key={hour.hour_start}><tr className="hourly-row">
          <th scope="row">{formatNewYorkTime(hour.hour_start, false)}</th>
          <td>{hour.forecast ? <>{formatNumber(hour.forecast.rain_mm)} mm · {hour.forecast.precipitation_probability}%
            {hour.forecast.snowfall_cm > 0 && <><br />{formatNumber(hour.forecast.snowfall_cm)} cm snow</>}</> : 'Unavailable'}</td>
          <td>{formatNumber(hour.baseline_trips)}</td><td>{formatNumber(hour.additional_trips)}</td>
          <td className="assessment">{describeHourAssessment(hour)}</td>
        </tr><tr className="mobile-evidence"><td colSpan={4}>{describeHourAssessment(hour)}</td></tr></Fragment>)}</tbody>
      </table>
    </div>
  </>;
}
