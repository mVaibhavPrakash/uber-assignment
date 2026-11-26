import { useCallback, useEffect, useRef, useState } from 'react';
import { getInsights, getRecommendation } from './api';
import type { Insights, Recommendation } from './types';

// Poll the API and hide saved recommendations as soon as their validity ends.
export function useRecommendation() {
  const [result, setResult] = useState<Recommendation | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [now, setNow] = useState(Date.now);
  const request = useRef<AbortController | null>(null);

  const refresh = useCallback(async () => {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setLoading(true);
    setError('');
    try {
      const next = await getRecommendation(controller.signal);
      if (!controller.signal.aborted) {
        setNow(Date.now());
        setResult(next);
      }
    } catch {
      if (!controller.signal.aborted) {
        setResult(null);
        setError('Could not load a current recommendation. Check the API and try again.');
      }
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
    const poll = window.setInterval(() => void refresh(), 60_000);
    return () => {
      window.clearInterval(poll);
      request.current?.abort();
    };
  }, [refresh]);

  useEffect(() => {
    if (!result || result.status === 'unavailable') return;
    const delay = Math.max(0, Date.parse(result.valid_until) - Date.now());
    const expiry = window.setTimeout(() => setNow(Date.now()), delay);
    return () => window.clearTimeout(expiry);
  }, [result]);

  const expired = !!result && result.status !== 'unavailable'
    && !(now < Date.parse(result.valid_until));

  useEffect(() => {
    if (expired) void refresh();
  }, [expired, refresh]);

  return { result, loading, error, expired, refresh };
}

// Fetch details for the same forecast and window as the main recommendation.
export function useInsights(
  result: Recommendation | null,
  areaID: number | null,
  expired: boolean,
  refresh: () => Promise<void>,
) {
  const [insights, setInsights] = useState<Insights | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    setInsights(null);
    setError('');
    if (!result || result.status === 'unavailable' || expired || areaID === null) return;
    const controller = new AbortController();
    void getInsights(areaID, result, controller.signal).then(next => {
      if (controller.signal.aborted) return;
      if (next === null) {
        void refresh();
      } else {
        setInsights(next);
      }
    }).catch(() => {
      if (!controller.signal.aborted) setError('Could not load area evidence. Try refreshing.');
    });
    return () => controller.abort();
  }, [result, areaID, expired, refresh]);

  // Never briefly show details from the previous zone or planning window.
  const current = insights?.evidence.area.id === areaID
    && insights.forecast_version === result?.forecast_version
    && insights.window.start === result?.window.start
    && insights.window.end === result?.window.end ? insights : null;
  return { insights: current, error };
}
