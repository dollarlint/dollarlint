import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  integrations: [
    starlight({
      title: 'dollarlint',
      description:
        'Validate JSON, YAML, and TOML files against the JSON Schema each file declares.',
      customCss: ['./src/styles/dollar.css'],
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: 'https://github.com/agorischek/dollarlint',
        },
      ],
      sidebar: [
        {
          label: 'Guides',
          items: [
            { label: 'Introduction', slug: 'guides/introduction' },
            { label: 'Installation', slug: 'guides/installation' },
            { label: 'Quick start', slug: 'guides/quick-start' },
            { label: 'Schema declarations', slug: 'guides/schema-declarations' },
            { label: 'SchemaStore', slug: 'guides/schemastore' },
            { label: 'Go SDK', slug: 'guides/go-sdk' },
          ],
        },
        {
          label: 'Reference',
          items: [
            { label: 'CLI', slug: 'reference/cli' },
            { label: 'Configuration', slug: 'reference/configuration' },
            { label: 'Output formats', slug: 'reference/output' },
            { label: 'Exit codes', slug: 'reference/exit-codes' },
          ],
        },
      ],
    }),
  ],
});
