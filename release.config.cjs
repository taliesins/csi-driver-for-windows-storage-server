const { execSync } = require('child_process');

const isDryRun = process.argv.includes('--dry-run');
const currentBranch =
  process.env.GITHUB_REF_NAME || execSync('git rev-parse --abbrev-ref HEAD').toString().trim();

module.exports = {
  branches: isDryRun ? [currentBranch] : ['main'],
  tagFormat: 'v${version}',
  plugins: [
    [
      '@semantic-release/commit-analyzer',
      {
        preset: 'conventionalcommits',
      },
    ],
    [
      '@semantic-release/release-notes-generator',
      {
        preset: 'conventionalcommits',
      },
    ],
    ...(isDryRun
      ? []
      : [
          '@semantic-release/changelog',
          [
            '@semantic-release/npm',
            {
              npmPublish: false,
            },
          ],
          [
            '@semantic-release/git',
            {
              assets: ['CHANGELOG.md', 'package.json', 'bun.lock'],
              message: 'chore(release): ${nextRelease.version}\n\n${nextRelease.notes}',
              commitArgs: '--no-verify',
            },
          ],
          '@semantic-release/github',
        ]),
  ],
};
