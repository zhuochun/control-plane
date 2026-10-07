# Publishing releases

1. Confirm the intended version, tag commit, previous release, and authorized
   publish scope. Read the changes between those tags and the tagged migrations,
   client contracts, and examples. Preserve unrelated local work.
2. Write concise notes covering the user-visible changes and required adoption.
   For stored `AGENTS.md` / `USER.md`, explain what the migration updates
   automatically, which customized values it preserves, and how clients compare
   recommended defaults, merge guidance, and save it. Mention MCP reconnection,
   copied prompt updates, and changed CLI/API/MCP payloads when applicable.
   Link references at the release tag. Label later `main` changes separately;
   never imply they are included in existing downloadable binaries.
3. Follow [the verification map](architecture.md#verification-path) and
   [.github/workflows/release.yml](../.github/workflows/release.yml).
   A tag push runs release verification and builds a draft through GoReleaser;
   a non-tag manual run builds a snapshot. For a new release, verify hosted
   checks, expected platform archives, `checksums.txt`, and package smoke results
   before publishing the draft. Report any unavailable evidence explicitly.
4. Use a UTF-8 notes file with `gh release edit <tag> --notes-file <path>`.
   Updating an existing description does not require rebuilding or retagging.
   Read back the description and verify its contents and links. For a new
   release, publish the verified draft within the user's authorized scope and
   confirm it is public; a successful local build or tag push is not publication.

Do not rewrite existing release tags during routine publishing. If explicitly
authorized history cleanup changes tags, explain the clone adoption impact and
whether existing release assets were retained or rebuilt. Keep release notes
free of private local paths and unsupported performance or compatibility claims.
