import { getRunRow } from '$lib/api/run';

export const load = async ({ fetch, params }) => {
  const result = await getRunRow(params.id, fetch);

  return result.match(
    (detail) => ({ detail, error: null }),
    (apiError) => ({ detail: null, error: apiError.message })
  );
};
