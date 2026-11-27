export interface Area {
    id: number;
    name: string;
}

export interface HourEvidence {
    hour_start: string;
    forecast: {
        rain_mm: number;
        snowfall_cm: number;
        precipitation_probability: number;
    } | null;
    dry_mean: number | null;
    rain_mean: number | null;
    dry_samples: number;
    rain_samples: number;
    baseline_trips: number | null;
    estimated_trips: number | null;
    additional_trips: number | null;
    reason?: string;
}

export interface AreaEvidence {
    area: Area;
    assessable: boolean;
    additional_trips_exact: number | null;
    baseline_trips: number | null;
    estimated_trips: number | null;
    additional_trips: number | null;
    relative_uplift_percent: number | null;
    reasons: string[] | null;
    hourly?: HourEvidence[];
}

export interface Recommendation {
    rainfall_threshold_mm: number;
    status: 'action_required' | 'no_intervention' | 'insufficient_evidence' | 'unavailable';
    decision_reason: string;
    message: string;
    freshness: string;
    window: { start: string; end: string; duration_seconds: number };
    generated_at: string;
    valid_until: string;
    forecast_version: string;
    forecast_fetched_at: string | null;
    coverage: { assessed: number; total: number };
    primary_recommendation: AreaEvidence | null;
    areas: AreaEvidence[];
    threshold_additional_trips_exact: number;
    threshold_additional_trips: number;
    highest_modeled_additional_trips: number | null;
    warnings: string[];
}

export interface Insights {
    forecast_version: string;
    window: Recommendation['window'];
    evidence: AreaEvidence;
}
