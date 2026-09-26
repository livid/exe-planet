# Platinum

A site template for [Planet](https://github.com/Planetable/Planet) that draws every page as one Mac OS 9 Platinum window on the desktop gray: the striped title bar carrying the page's name, the sunken frame, a row of push buttons for Home, Archive, Tags, the site's pages and RSS, and a status strip with the counts or the date. It reads the same context as the Plain template, so it builds a paginated index, posts, pages, the archive by month, the tag list and tag pages.

It comes from [exe](https://exe.v2core.com/), whose desktop, homepage and hub wear the same chrome. `assets/chrome.css` is a verbatim copy of that shared block (the window, its striped bar, the sunken frame, the status strip, the push button); re-copy it whole when it moves, never edit it here. `assets/style.css` is the site's own.

`assets/rss.xsl` styles the feed for a browser: exe's Planet writes the `xml-stylesheet` line into `rss.xml` for a template that ships this file, and a browser that still transforms XSLT draws the feed as one more window. The Planet app does not write that line, and a feed reader never sees the stylesheet either way.

A post can carry its replies from an [exe hub](https://github.com/livid/exe-hub): when exe's Planet has announced the post on a hub, the post's page gets a Reply window and, under it, a Replies window framing the hub's own replies page for that post, which draws the rows live and in the reader's language. The reader answers in the Reply window with a Solana wallet, which signs a hub message and never a transaction; `assets/replies.js` is that composer, and it runs only on such a page. The Planet app never announces a post, so on a site it builds these windows never appear.

The first site to wear it is [blog.v2core.com](https://blog.v2core.com/).
