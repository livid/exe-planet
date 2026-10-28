# Cloud

A site template for [Planet](https://github.com/Planetable/Planet): a white page and a frosted sidebar over a painted sky. It is the look of the [CodeLauncher](https://cl.v2ex.pro/) website of 2022, made for a product with a blog. The sky is "clouds", which Mia painted for that site.

The sidebar holds the site's name, its pages in the navigation, Blog and, when any post has tags, Tags. On a phone the sidebar becomes one row at the top of the page.

The home page shows the site's avatar beside its name, then the About in full, then the posts under "Blog". Write the About in Markdown: it can hold headings, lists, pictures and HTML, so it can be the product's front page. The second page of posts and the rest are the list alone. Blog in the sidebar opens the archive, all posts in one list.

A post has a breadcrumb back to the site and the Blog, its title, its date and its text. A page in the navigation has only its title and text.

It reads the same context as the Plain template, so it builds a paginated index, posts, pages, the archive, the tag list and tag pages.

A post can name its own author, when it is built by exe's Planet: the front matter lines `author`, `author_link` and `author_avatar` (a picture in the post's folder) put the author's name and picture beside the post's date and in the lists. The Planet app does not set them, so on a site it builds the date stands alone.

It comes from [exe](https://exe.v2core.com/).
