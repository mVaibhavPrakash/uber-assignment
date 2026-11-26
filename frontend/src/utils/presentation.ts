import type { HourEvidence, Recommendation } from '../types/recommendation';

export function formatNumber(value: number | null | undefined) {
  return value == null ? 'Not available' : value.toLocaleString('en-US', { maximumFractionDigits: 1 });
}

export function formatSignedNumber(value: number | null | undefined) {
  return value == null ? 'Not available' : `${value > 0 ? '+' : ''}${formatNumber(value)}`;
}

export function formatNewYorkTime(value: string, includeDate = true) {
  return new Intl.DateTimeFormat('en-US', {
    timeZone: 'America/New_York',
    ...(includeDate ? { month: 'short', day: 'numeric' } as const : {}),
    hour: 'numeric', minute: '2-digit',
  }).format(new Date(value));
}

export function formatPlanningDuration(result: Recommendation) {
  const hours = result.window.duration_seconds / 3600;
  return `${formatNumber(hours)} ${hours === 1 ? 'hour' : 'hours'}`;
}

export function formatAreaName(name: string) {
  return name.replace(/\s*\(([^)]+)\)$/, ', $1');
}

// Explain the backend's assessment without reimplementing eligibility.
export function describeHourAssessment(hour: HourEvidence, rainThreshold: number) {
  switch (hour.reason) {
    case 'missing_forecast': case 'invalid_forecast': return 'Forecast unavailable';
    case 'snow_not_supported': return 'Unsupported weather: snow';
    case 'missing_history': case 'insufficient_dry_history': case 'insufficient_rain_history':
      return 'Insufficient historical evidence';
  }
  if (!hour.forecast) return 'Forecast unavailable';
  if (hour.forecast.rain_mm < rainThreshold) {
    return hour.baseline_trips == null
      ? 'Below rain threshold. Typical trips unavailable.'
      : 'Below rain threshold';
  }
  return 'Estimate available';
}
