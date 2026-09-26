<?xml version="1.0" encoding="UTF-8"?>
<!-- The feed, as a browser shows it: rss.xml names this stylesheet and a
     browser that still transforms XSLT draws the feed as one more
     Platinum window, the site's name in the bar, the posts as the index
     lists them, the address to paste into a reader. A reader never sees
     any of this; it reads the XML. -->
<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform" xmlns:atom="http://www.w3.org/2005/Atom">
  <xsl:output method="html" encoding="UTF-8" indent="no"/>
  <xsl:template match="/">
    <xsl:variable name="title" select="/rss/channel/title"/>
    <xsl:variable name="home" select="/rss/channel/link"/>
    <xsl:variable name="self" select="/rss/channel/atom:link[@rel='self']/@href"/>
    <xsl:variable name="about" select="normalize-space(/rss/channel/description)"/>
    <xsl:variable name="n" select="count(/rss/channel/item)"/>
    <html lang="en">
      <head>
        <meta charset="UTF-8"/>
        <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover"/>
        <meta name="theme-color" content="#cccccc"/>
        <title><xsl:value-of select="$title"/> — Feed</title>
        <link rel="stylesheet" href="assets/chrome.css"/>
        <link rel="stylesheet" href="assets/style.css"/>
      </head>
      <body>
        <div class="window">
          <div class="titlebar"><div class="stripe"></div><div class="title"><xsl:value-of select="$title"/> — Feed</div><div class="stripe"></div></div>
          <div class="frame">
            <div class="strip tools"><div class="crumbs"><span class="crumb"><a href="{$home}"><xsl:value-of select="$title"/></a></span><svg class="sep" viewBox="0 0 4 7" aria-hidden="true"><path class="k" d="M0 0h1v1H0zM1 1h1v1H1zM2 2h1v1H2zM3 3h1v1H3zM2 4h1v1H2zM1 5h1v1H1zM0 6h1v1H0z"/></svg><b>Feed</b></div></div>
            <div class="body">
              <div class="head">
                <h1>
                  <xsl:value-of select="$title"/>
                  <small>
                    <xsl:choose>
                      <xsl:when test="contains($about, '&lt;p&gt;')"><xsl:value-of select="substring-before(substring-after($about, '&lt;p&gt;'), '&lt;/p&gt;')"/></xsl:when>
                      <xsl:otherwise><xsl:value-of select="$about"/></xsl:otherwise>
                    </xsl:choose>
                  </small>
                </h1>
              </div>
              <p class="feed-note">This is the site's feed, an RSS file. Paste its address into a feed reader and new posts come to you: <code><xsl:value-of select="$self"/></code></p>
              <div class="items">
                <xsl:for-each select="/rss/channel/item">
                  <div class="item">
                    <div class="item-head">
                      <a class="item-title" href="{link}">
                        <xsl:choose>
                          <xsl:when test="string-length(normalize-space(title)) &gt; 0"><xsl:value-of select="title"/></xsl:when>
                          <xsl:otherwise><xsl:value-of select="substring(pubDate, 6, 11)"/></xsl:otherwise>
                        </xsl:choose>
                      </a>
                      <span class="item-date"><xsl:value-of select="substring(pubDate, 6, 11)"/></span>
                    </div>
                  </div>
                </xsl:for-each>
              </div>
            </div>
            <div class="btnrow"><a class="btn default" href="{$home}">Home</a></div>
            <div class="statusbar"><span><xsl:value-of select="$n"/><xsl:text> </xsl:text><xsl:choose><xsl:when test="$n = 1">post</xsl:when><xsl:otherwise>posts</xsl:otherwise></xsl:choose></span><span>RSS 2.0</span></div>
          </div>
        </div>
        <p class="foot">Built with <a href="https://exe.v2core.com/">exe</a> and its Planet.</p>
      </body>
    </html>
  </xsl:template>
</xsl:stylesheet>
