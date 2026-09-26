# Platinum

A site template for [Planet](https://github.com/Planetable/Planet) that draws every page as one Mac OS 9 Platinum window on the desktop gray: the striped title bar carrying the page's name, the sunken frame, a row of push buttons for Home, Archive, Tags, the site's pages and RSS, and a status strip with the counts or the date. It reads the same context as the Plain template, so it builds a paginated index, posts, pages, the archive by month, the tag list and tag pages.

It comes from [exe](https://exe.v2core.com/), whose desktop, homepage and hub wear the same chrome. `assets/chrome.css` is a verbatim copy of that shared block (the window, its striped bar, the sunken frame, the status strip, the push button); re-copy it whole when it moves, never edit it here. `assets/style.css` is the site's own.

`assets/rss.xsl` styles the feed for a browser: exe's Planet writes the `xml-stylesheet` line into `rss.xml` for a template that ships this file, and a browser that still transforms XSLT draws the feed as one more window. The Planet app does not write that line, and a feed reader never sees the stylesheet either way.

The first site to wear it is [blog.v2core.com](https://blog.v2core.com/).
