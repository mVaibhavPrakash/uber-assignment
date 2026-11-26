import type { Recommendation } from '../types/recommendation';

// Keep warnings focused on the information a controller needs before acting.
function buildDecisionWarnings(result: Recommendation) {
  const items = [{ title: 'Weather changes', text: result.freshness === 'stale'
    ? 'The forecast is older than the normal refresh interval. Check current local conditions before acting.'
    : 'Unexpected rainfall or local differences within a zone could change the estimated trip volume.' }];
  if (result.status === 'unavailable') {
    items.push({ title: 'Missing current context', text: 'A complete current forecast or recommendation is unavailable. No action is being recommended.' });
  } else if (result.coverage.assessed < result.coverage.total) {
    items.push({ title: 'Incomplete evidence', text: `Only ${result.coverage.assessed} of ${result.coverage.total} zones can be assessed. Decisions cover assessed zones only; a higher-impact zone may be among those with incomplete evidence or unsupported weather.` });
  }
  items.push({ title: 'Unknown driver availability', text: 'Completed trips do not measure unmet demand or available drivers. Check current supply before taking action.' });
  items.push({ title: 'Changing travel patterns', text: 'Past completed trips may not reflect current behavior, holidays, events, pricing or disruptions.' });
  return items;
}

export default function Uncertainty({ result }: { result: Recommendation }) {
  return <section className="uncertainty" aria-labelledby="uncertainty-title">
    <h2 id="uncertainty-title"><span aria-hidden="true">△</span> What could change this decision?</h2>
    <ul className="context-warnings">{buildDecisionWarnings(result).map(item =>
      <li key={item.title}><strong>{item.title}:</strong> {item.text}</li>)}</ul>
  </section>;
}
