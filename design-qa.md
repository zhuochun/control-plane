# Reader workspace design QA

final result: passed

Implemented selected Option 1 with the user's refinements: no duplicate reader banner or Expand action; complete report in the reader; note composer inline after short reports and pinned below overflowing reports.

## Latest annotation refinement

### Workspace and inbox follow-up

Removed the duplicate queue view links. New Item is now **Add to inbox**, a quick single-field capture for a note, link, or instruction. It creates a user-owned note (without a Todo); the saved `item.created` user event is already delivered in the next Run's change feed. An optional title and source date sit under Optional context. The final screenshot `14-final-inbox-workspace.png` shows the clean Library reader. The capture regression confirms full text is retained and the created Item event appears in `/api/v1/brief`.

The top bar now wraps before its controls exceed tablet widths. The responsive check covers 390, 780, 1100, and 1440px across Library, Monitoring, Activity, and Preferences, checking horizontal overflow and page height. At 900px tall, Library's document height is exactly 900px; Monitoring and Activity stay within their available page height. Preferences scrolls where its actual context editors extend beyond the viewport. Added `display: flow-root` to `main` to keep child margins within the viewport calculation and remove an unnecessary 32px page scroll on shorter sections. Report and queue panes retain their own scroll when their content exceeds their panes.

The user's note about vertical scroll clarified a separate issue from the reader: extra document scroll on short Workspace pages. The 16-browser-test rerun passes with assertions for both horizontal overflow and unnecessary page scroll, plus inbox handoff, note placement, and filters. The web/TypeScript and executable builds pass; `git diff --check` is clean.

The user's annotations supersede the original mock where applicable. Removed the visible queue title/count toolbar and moved the Add to inbox action to the top navigation. A visually hidden heading retains the queue's accessible name. Filters have a 12px inset and 10px control gap; explicit border-box sizing fixes native selects exceeding their grid tracks. The reader content now shrinks when necessary but does not grow to consume unused space, so the note stays inline for short reports and at the viewport bottom for long reports.

Final evidence in the directory below:

- `12-annotations-inline-final.png`: 2265 x 1244 pixels/CSS viewport, density 1, matching the annotated viewport and short Deployment follow-up report. Visible queue toolbar is absent, New Item is in the top bar, expanded filter controls have a measured 10px gap and 13px right inset. Report scroll height equals client height (527px); the note ends at y=727 rather than the viewport bottom.
- `13-annotations-long-report.png`: 1440 x 700 pixels/CSS viewport, density 1, Automation cost report. Report scroll height is 1493px in a 500px visible area; the note ends at y=700 and remains available.
- `11-annotations-inline.png`: intermediate capture exposed native select content-box overflow. Corrected and recaptured as `12-annotations-inline-final.png` before passing.
- `14-final-inbox-workspace.png`: final top bar, queue, and reader after removing queue view links and replacing New Item.

Visual checks confirmed the annotation intent; typography, colors, icons, and report content are unchanged. There are no actionable P0/P1/P2 findings from this refinement. Browser console has zero errors. Web build/TypeScript check, executable build, all 16 browser tests, and diff checks passed. The browser coverage includes inline/pinned transitions on resize, draft retention, inbox event delivery, top-bar capture entry, filter containment, and horizontal/vertical scroll behavior. An existing shared-database count assertion was made tolerant of concurrent Item creation by other test workers; actual captured Item assertions remain.

The full Windows gate recorded below passed for the original reader implementation. Go behavior is unchanged in these frontend refinements.

## Visual evidence

Source visual truth: `C:/Users/zhuoc/.codex/generated_images/01a0f1c1-4937-7043-b66a-9522d4a02647/exec-9f011ad7-fe92-4b51-9d48-e0cc13d381b0.png`.

Implementation: <http://127.0.0.1:7331/> using an isolated temporary database with fictional preview data. Route `/`, Attention, selected "Automation cost fell 18%", light theme, empty note at top of report.

Evidence directory: `C:/Users/zhuoc/.codex/visualizations/2026/09/30/01a0f1c1-4937-7043-b66a-9522d4a02647/`.

- Final desktop capture: `06-reader-desktop-final.png`.
- Combined source/implementation full-view comparison: `07-comparison-final.png`.
- Focused combined chart comparison: `comparison-detail-2.png`.
- Focused combined note composer comparison: `comparison-detail-3.png`.
- Desktop report scrolled to complete table, sources, and relevance with an editable note: `08-reader-scrolled-note.png`.
- Final mobile capture: `10-reader-mobile-final.png`.

Desktop source and final screenshot are both 1487 x 1058 pixels, matching a 1487 x 1058 CSS viewport at devicePixelRatio 1. No final density normalization. The initial screenshot was 1472 x 1047 and was normalized to source dimensions for the first combined comparison; exact DOM measurements separately established the overflow defect. Mobile was 390 x 844 CSS and image pixels, density 1. Mobile has no separate source mockup and was evaluated for usability rather than pixel fidelity.

## Comparison history

1. Initial comparison (`05-comparison-before.png`, based on `04-reader-desktop.png`) was blocked:
   - P1: The chart treated "$2 above target" as an absolute target. The metric readout and dashed reference therefore misrepresented the report. Prioritized the explicit target, rejected relative above/below phrases, and constrained numeric-unit whitespace to the current line. Added browser regression cases for explicit target and absent absolute target.
   - P2: Queue width and heading sizes were too small relative to the selected direction. Increased desktop queue to a responsive 350-410px track, reader title to 36px, and report text to 17px.
   - P2: Observations lacked visible value labels. Added chart observation labels and solid points, preserving accessible canvas text and complete Markdown table.
   - P2: Desktop outer scrollbar came from a 73px topbar combined with viewport height calculated for 72px. Included its border within the 72px height. Post-fix document height equals the viewport.
2. Revised desktop comparison (`07-comparison-final.png` plus focused regions) confirms correct $82 latest, -18% change, $80 target, dated observations, appropriate queue proportions, and the note composer below the report scroll area. No actionable desktop P0/P1/P2 findings remain.
3. Mobile capture `09-reader-mobile.png` exposed another P2: the main minimum height still used the desktop header height, producing an extra page scrollbar. Matched the mobile main minimum height to its 124px header. `10-reader-mobile-final.png` and DOM measurements confirm document width/height equal 390 x 844, one report scrollbar, and note composer top at y=728 with bottom at y=844.

## Required fidelity surfaces

- Fonts and typography: Preserved the repository's Inter/Segoe UI/sans-serif stack; Segoe UI is the available fallback on this Windows preview. The generated source appears closer to Inter and has a slightly darker optical weight. Reader title, 42px desktop metrics, section headings, and larger body text establish the intended hierarchy. Source typography is inspiration rather than a supplied font asset; exact raster antialiasing is not an acceptance claim.
- Spacing and layout rhythm: Full-width 72px navigation, approximately 410px queue, 32px reader inset, separate queue/report scroll containers, and pinned note composer match the composition. No duplicate banner or Expand affordance. Mobile shows a full-screen reader with Back to items and persistent note controls.
- Colors and tokens: White reading surfaces, existing green primary/selection tokens, subdued text, light dividers, and warm Todo badges retain the chosen restrained palette. Focus outlines and active queue selection remain visible. Disabled Save note correctly indicates an unchanged empty note.
- Image quality and asset fidelity: The existing aicp brand mark is retained. Standard source/open/back/search/menu icons use Material icons. There are no new raster illustrations. The chart is rendered from report values with Chart.js, not a mockup screenshot or decorative image.
- Copy and content: The selected title, summary, metric, interpretation, and evidence are displayed. All report Markdown, tables, sources, report actions, and Interest rationale remain accessible in the reader. No fixture text is baked into the product. Current Item update timestamp is labeled "Updated" rather than calling it a source timestamp. Comparison label uses the actual $100 baseline instead of inventing a time period.

## Accepted differences and limits

The generated mock omits the complete observations table and Interest rationale. The implementation retains them, which moves sources below the first fold; the scrolled evidence confirms they are available without opening another page. Queue order, attention membership, Todo counts, and reminders come from current API semantics rather than the illustrative mock. Two summary lines and explicit unsaved-note feedback increase queue row height. New Item and collapsed Filters preserve existing functions. These are intentional functional adaptations.

P3 follow-up polish: exact font weight and queue badge alignment can be tuned after user review. Chart scale uses the values' automatic axis range rather than the mock's fixed $60-$120 ticks. The build emits a bundle-size advisory (about 904 kB minified / 285 kB gzip); loading performance was not benchmarked.

Mobile virtual-keyboard behavior on a physical device remains unverified. Drafts are kept in memory across item switches; persistence of an unsaved draft after page reload is not promised.

## Interaction and verification evidence

- Full Windows `scripts/verify.ps1` passed: dependency installation/audit (0 vulnerabilities), TypeScript check, web build, Go vet/tests, executable build, all 13 browser tests, and two-cycle fixture demo.
- After the final mobile-only CSS correction, web build (including TypeScript), executable build, and all 13 browser tests passed again.
- Browser tests cover full content/actions/source links, Todo/reminder lifecycle, pinned editor on desktop/mobile, switching drafts, save/reload, edits during delayed saves, and acknowledgement retaining the open report.
- Live browser check: desktop note remains at y=930 before and after report scrollTop=635; complete table and sources are visible below; mobile has no horizontal or outer-page overflow.
- Clean final preview tab console checked on desktop and mobile: zero error entries. Earlier diagnostic-tab errors from superseded builds were fixed before this check.
- Independent read-only code review completed. All actionable findings were fixed; reviewer did not claim to run the suite or visually review the UI.
- `git diff --check` passed. Unrelated pre-existing docs and style cleanup were preserved.

## Implementation checklist

- [x] Replace persistent left navigation with top navigation and compact queue.
- [x] Display complete details and larger metric/chart in the reader.
- [x] Remove duplicate reader banner and Expand.
- [x] Pin note editor outside scroll and preserve drafts across item switches.
- [x] Compare source and rendered implementation in combined images; fix P1/P2 defects and recapture.
- [x] Verify responsive behavior and browser console.
- [x] Complete automated verification and independent code review.
