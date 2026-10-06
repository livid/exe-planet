# Paper

A site template for [Planet](https://github.com/Planetable/Planet): one quiet column of Noto Serif on grey paper grain, made for literature and the humanities. No boxes, no shadows; the site's name and a few ways around it in the quiet ink, the posts as a list with their dates in figures, and a hairline only where a line says something. The ground is drawn by an SVG noise filter, so the grain stays fine at every pixel ratio, and the page follows the reader's light or dark.

The Language setting (`zh-Hans`, the default; `zh-Hant`, `ja` or `en`) is the page's `lang`, and the rest follows it:

- the face: Noto Serif SC, TC or JP first on a Chinese or Japanese site, so full-width punctuation and the long dash come from the CJK face, and Noto Serif first on an English one, all from Google Fonts;
- the words around the text (归档, 標籤, アーカイブ, Archive …), from the `w_*.html` modules;
- the dates, written for that language by a small script at the end of each page;
- CJK typography: justified text, strict line breaking, a thin space where a Han character meets a Latin letter or a figure, and emphasis as dots under the characters in Chinese (着重号) or sesame dots over them in Japanese. A paragraph written in Latin letters alone is set as English: Noto Serif, ragged right, real italics. On an English site it is the other way round: a paragraph that is mostly Chinese or Japanese is set as that, in the CJK face at Medium, justified, with its emphasis as dots.

It reads the same context as the Plain template, so it builds a paginated index, posts, pages, the archive by month, the tag list and tag pages.

A post can carry its replies from an [exe hub](https://github.com/livid/exe-hub), as the Platinum template's do: when exe's Planet has announced the post on a hub, the post's page gets the Reply composer and, under it, the hub's replies page for that post in a frame, asked for in the paper look (`?look=paper`), so the hub draws the rows on this same paper in the same type, and in the site's language. The reader answers with a Solana wallet, which signs a hub message and never a transaction; `assets/replies.js` is Platinum's composer with its words in the site's language. The Planet app never announces a post, so on a site it builds these never appear.

It comes from [exe](https://exe.v2core.com/).
