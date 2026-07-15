import { getRuns } from '$lib/api/runs';

export const load = async ({ fetch }) => {
  const result = await getRuns(fetch);

  return result.match(
    (runs) => ({ runs, error: null }),
    (apiError) => ({ runs: [], error: apiError.message })
  );
};
