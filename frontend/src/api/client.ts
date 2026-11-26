import type { Insights, Recommendation } from '../types/recommendation';

export async function fetchRecommendation(signal: AbortSignal): Promise<Recommendation> {
  const response = await fetch('/api/recommendations', { signal, cache: 'no-store' });
  // An unavailable recommendation still has a useful status and explanation.
  if (!response.ok && response.status !== 503) {
    throw new Error('Could not load the recommendation. Please try again.');
  }
  return response.json();
}

export async function fetchMatchingInsights(
  areaID: number,
  result: Recommendation,
  signal: AbortSignal,
): Promise<Insights | null> {
  const query = new URLSearchParams({
    forecast_version: result.forecast_version,
    window_start: result.window.start,
  });
  const response = await fetch(`/api/areas/${areaID}/insights?${query}`, {
    signal,
    cache: 'no-store',
  });
  
  // A new forecast or window requires reloading the main recommendation first.
  if (response.status === 409) {
    return null;
  }
  if (!response.ok) {
    throw new Error('Area evidence is unavailable. Try refreshing.');
  }

  const details: Insights = await response.json();
  if (details.forecast_version !== result.forecast_version|| details.window.start !== result.window.start || details.window.end !== result.window.end) {
    return null;
  }

  return details;
}
