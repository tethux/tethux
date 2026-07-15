import { getTests } from '$lib/api/tests';

export const load = async ({ fetch }) => {
  const result = await getTests(fetch);

  return result.match(
    (tests) => ({ tests, error: null }),
    (apiError) => ({ tests: [], error: apiError.message })
  );
};
