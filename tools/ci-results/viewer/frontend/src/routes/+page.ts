import { getRuns } from '$lib/api/runs';
import { getSummary } from '$lib/api/summary';

export const load = async ({ fetch }) => {
  const [summaryResult, runsResult] = await Promise.all([getSummary(fetch), getRuns(fetch)]);
  const errors: string[] = [];

  const summary = summaryResult.match(
    (value) => value,
    (error) => {
      errors.push(error.message);
      return null;
    }
  );
  const runs = runsResult.match(
    (value) => value,
    (error) => {
      errors.push(error.message);
      return [];
    }
  );

  return { summary, runs, error: errors.join('; ') || null };
};
