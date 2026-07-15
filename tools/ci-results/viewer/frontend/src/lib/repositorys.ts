// src/lib/

export const sourceRepositories = [
  {
    name: 'Codeberg',
    url: 'https://codeberg.org/your-name/your-repository'
  },
  {
    name: 'GitHub',
    url: 'https://github.com/your-name/your-repository'
  }
] satisfies Array<{
  name: string;
  url: string;
}>;
