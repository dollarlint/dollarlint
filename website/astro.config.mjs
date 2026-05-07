import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://agorischek.github.io',
  base: '/dollarlint',
  integrations: [
    starlight({
      title: 'dollarlint',
      description:
        'Check your JSON, YAML, and TOML files against their `$schema`s, both locally and in CI.',
      logo: {
        light: './src/assets/logo-light.svg',
        dark: './src/assets/logo-dark.svg',
      },
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: 'https://github.com/agorischek/dollarlint',
        },
      ],
      customCss: ['./src/styles/dollar.css'],
      sidebar: [
        { label: 'Overview', slug: 'index' },
        {
          label: 'Use dollarlint',
          items: [
            { label: 'Getting started', slug: 'guides/getting-started' },
            { label: 'Configuration', slug: 'guides/configuration' },
            { label: 'Output formats', slug: 'guides/output-formats' },
            { label: 'CI integration', slug: 'guides/ci' },
            { label: 'Go SDK', slug: 'guides/go-sdk' },
          ],
        },
      ],
      editLink: {
        baseUrl: 'https://github.com/agorischek/dollarlint/edit/main/website/',
      },
    }),
  ],
});
