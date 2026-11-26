import { Fragment } from 'react';
import ComparisonBars from './ComparisonBars';
import { areaName, assessment, date, duration, number } from './presentation';
import type { AreaEvidence, Recommendation } from './types';

export default function EvidencePanel({ evidence, result }: { evidence: AreaEvidence; result: Recommendation }) {
  return <>
    <div className="evidence-card"><h3>Expected trips over the next {duration(result)}</h3>
      <ComparisonBars evidence={evidence} /></div>
    <h3>Hourly breakdown</h3>
    <div className="table-scroll" tabIndex={0} role="region" aria-label={`${areaName(evidence.area.name)} hourly evidence`}>
      <table className="hourly-table">
        <thead><tr><th scope="col">Time</th><th scope="col">Rain forecast</th>
          <th scope="col">Typical trips</th><th scope="col">Extra trips</th><th scope="col">Evidence</th></tr></thead>
        <tbody>{evidence.hourly?.map(hour => <Fragment key={hour.hour_start}><tr className="hourly-row">
          <th scope="row">{date(hour.hour_start, false)}</th>
          <td>{hour.forecast ? <>{number(hour.forecast.rain_mm)} mm · {hour.forecast.precipitation_probability}%
            {hour.forecast.snowfall_cm > 0 && <><br />{number(hour.forecast.snowfall_cm)} cm snow</>}</> : 'Unavailable'}</td>
          <td>{number(hour.baseline_trips)}</td><td>{number(hour.additional_trips)}</td>
          <td className="assessment">{assessment(hour)}</td>
        </tr><tr className="mobile-evidence"><td colSpan={4}>{assessment(hour)}</td></tr></Fragment>)}</tbody>
      </table>
    </div>
  </>;
}
