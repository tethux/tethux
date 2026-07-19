import { getSchemaInfo } from '$lib/api/querys';

export const load = async ({ fetch }) => {
	const result = await getSchemaInfo(fetch);

	return result.match(
		(schemaInfo) => ({
			schemaInfo,
			error: null
		}),
		(apiError) => ({
			schemaInfo: null,
			error: apiError.message
		})
	);
};
