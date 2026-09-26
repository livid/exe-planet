#!/usr/bin/env python3
"""The social card: a 1200 by 675 animated GIF (the size X shows in the
timeline) that carries the 88 by 31 badge blown up pixel for pixel and
the introduction beside it, drawn as one Platinum window at double
scale: 2px black edges, 2px bevels, 2px title-bar stripes, the desk gray
around it. The badge's eight frames are the only motion.

    FONT_DIR=<folder with Geneva.ttf and Monaco.ttf> python3 make_card.py [out.gif]

Reads exe-planet-88x31.gif from its own folder. The era's own faces are
Apple's: Charcoal for the system font (title bar, headline), Geneva for
the application font (the text) and Monaco for the monospace. Geneva and
Monaco still ship in macOS (/System/Library/Fonts); Charcoal only lives
in a Mac OS 9 System Folder, so it is used when a Charcoal.ttf is in
FONT_DIR and Geneva stands in otherwise. Without FONT_DIR the DejaVu
faces stand in for all three. Fonts are never kept in this repo."""
import os
import sys

from PIL import Image, ImageDraw, ImageFont, ImageSequence

HERE = os.path.dirname(os.path.abspath(__file__))
BADGE = os.path.join(HERE, "exe-planet-88x31.gif")
OUT = sys.argv[1] if len(sys.argv) > 1 else os.path.join(HERE, "exe-planet-card-1200x675.gif")
FONT_DIR = os.environ.get("FONT_DIR", "")
DEJAVU = "/usr/share/fonts/truetype/dejavu/"


def face(names, size, fallback):
    """The first of names found in FONT_DIR, else the DejaVu fallback."""
    for n in names:
        p = os.path.join(FONT_DIR, n)
        if FONT_DIR and os.path.exists(p):
            return ImageFont.truetype(p, size)
    return ImageFont.truetype(DEJAVU + fallback, size)


SYSTEM = ["Charcoal.ttf", "Charcoal.otf", "Geneva.ttf"]
APP = ["Geneva.ttf"]
MONO = ["Monaco.ttf"]

W, H = 1200, 675
S = 2  # the Platinum scale: every 1px line of the desk is 2px here
BLACK = (0x26, 0x26, 0x26)
G200, G300, G400, G600, G700 = (0xEE,) * 3, (0xDD,) * 3, (0xCC,) * 3, (0x99,) * 3, (0x77,) * 3
WHITE, HL, LAV, QUIET = (0xFF,) * 3, (0x33, 0x33, 0x99), (0xCC, 0xCC, 0xFF), (0x55,) * 3

bold = face(SYSTEM, 54, "DejaVuSans-Bold.ttf")
body = face(APP, 27, "DejaVuSans.ttf")
small = face(APP, 22, "DejaVuSans.ttf")
mono = face(MONO, 22, "DejaVuSansMono.ttf")
title_font = face(SYSTEM, 24, "DejaVuSans-Bold.ttf")
# Geneva has no bold; where it stands in for Charcoal the title and the
# headline get a one-pixel stroke, the way the desk fakes it too
HEAVY = 1 if not (FONT_DIR and os.path.exists(os.path.join(FONT_DIR, "Charcoal.ttf"))) else 0


def rect(d, x0, y0, x1, y1, fill=None, outline=None, width=S):
    d.rectangle([x0, y0, x1 - 1, y1 - 1], fill=fill, outline=outline, width=width if outline else 0)


def window(d, x, y, w, h, title):
    """A Platinum window at scale S: edge, bevel, striped bar with the
    title on a field, the seam, and a white frame below. Answers the
    content box (x0, y0, x1, y1)."""
    # the window face and its edge
    rect(d, x, y, x + w, y + h, fill=G400, outline=BLACK)
    # bevel: white top-left, dark bottom-right, inside the edge
    d.rectangle([x + S, y + S, x + w - S - 1, y + S * 2 - 1], fill=WHITE)
    d.rectangle([x + S, y + S, x + S * 2 - 1, y + h - S - 1], fill=WHITE)
    d.rectangle([x + S * 2, y + h - S * 2, x + w - S - 1, y + h - S - 1], fill=G600)
    d.rectangle([x + w - S * 2, y + S * 2, x + w - S - 1, y + h - S - 1], fill=G600)
    # the title bar: stripes of 2px white / 2px #777 on #ddd, the title on a field
    bar_y0, bar_y1 = y + S * 2 + 2 * S, y + S * 2 + 2 * S + 13 * S
    bx0, bx1 = x + S * 4, x + w - S * 4
    d.rectangle([bx0, bar_y0, bx1 - 1, bar_y1 - 1], fill=G300)
    for yy in range(bar_y0 + S, bar_y1 - S, 2 * S):
        d.rectangle([bx0, yy, bx1 - 1, yy + S - 1], fill=WHITE)
        d.rectangle([bx0, yy + S, bx1 - 1, yy + 2 * S - 1], fill=G700)
    # the close box at the left, 13 units square
    cx, cy = bx0 + S * 2, bar_y0
    d.rectangle([cx, cy, cx + 13 * S - 1, cy + 13 * S - 1], fill=G400)
    d.rectangle([cx + S, cy + S, cx + 12 * S - 1, cy + 12 * S - 1], outline=BLACK, width=S)
    d.rectangle([cx + 3 * S, cy + 3 * S, cx + 10 * S - 1, cy + 10 * S - 1], fill=WHITE)
    d.rectangle([cx + 3 * S, cy + 3 * S, cx + 9 * S - 1, cy + 9 * S - 1], outline=G700, width=S)
    tw = d.textlength(title, font=title_font) + 2 * HEAVY
    tx = x + (w - tw) / 2
    d.rectangle([tx - 8, bar_y0, tx + tw + 8, bar_y1 - 1], fill=G400)
    d.text((tx, bar_y0 + (13 * S - 24) / 2 - 2), title, font=title_font, fill=BLACK, stroke_width=HEAVY, stroke_fill=BLACK)
    # the frame: a white field inside a black line, sunk with a bevel
    fx0, fy0 = x + S * 4, bar_y1 + S * 3
    fx1, fy1 = x + w - S * 4, y + h - S * 4
    d.rectangle([fx0 - S, fy0 - S, fx1 - 1, fy1 - 1], fill=G600)  # top-left shade
    d.rectangle([fx0, fy0, fx1 + S - 1, fy1 + S - 1], fill=WHITE)  # bottom-right light
    d.rectangle([fx0, fy0, fx1 - 1, fy1 - 1], fill=WHITE, outline=BLACK, width=S)
    return fx0 + S, fy0 + S, fx1 - S, fy1 - S


def statusbar(d, x0, y0, x1, y1, left, right):
    d.rectangle([x0, y0, x1 - 1, y1 - 1], fill=G300)
    d.rectangle([x0, y0, x1 - 1, y0 + S - 1], fill=BLACK)
    d.rectangle([x0, y0 + S, x1 - 1, y0 + 2 * S - 1], fill=(0xF1,) * 3)
    d.text((x0 + 16, y0 + (y1 - y0 - 22) / 2 + S), left, font=small, fill=(0x33,) * 3)
    tw = d.textlength(right, font=small)
    d.text((x1 - 16 - tw, y0 + (y1 - y0 - 22) / 2 + S), right, font=small, fill=(0x33,) * 3)


def wrap(d, text, font, width):
    words, lines, line = text.split(), [], ""
    for w in words:
        t = (line + " " + w).strip()
        if d.textlength(t, font=font) <= width:
            line = t
        else:
            lines.append(line)
            line = w
    if line:
        lines.append(line)
    return lines


def base_frame():
    im = Image.new("RGB", (W, H), G400)
    d = ImageDraw.Draw(im)
    cx0, cy0, cx1, cy1 = window(d, 40, 36, W - 80, H - 72, "exe-planet")
    # the status strip along the frame's bottom
    sb_h = 30
    statusbar(d, cx0 - S, cy1 - sb_h, cx1 + S, cy1 + S, "blog.v2core.com", "Open source, one Go binary, on a machine you own")
    # top left: the badge at 6x on a lavender field, the well a sunken box
    scale = 6
    bw, bh = 88 * scale, 31 * scale
    pad = 20
    wx0, wy0 = cx0 + 44, cy0 + 40
    wx1, wy1 = wx0 + bw + 2 * pad, wy0 + bh + 2 * pad
    d.rectangle([wx0 - S, wy0 - S, wx1 - 1, wy1 - 1], fill=G600)
    d.rectangle([wx0, wy0, wx1 + S - 1, wy1 + S - 1], fill=WHITE)
    d.rectangle([wx0, wy0, wx1 - 1, wy1 - 1], fill=LAV, outline=BLACK, width=S)
    badge_at = (wx0 + pad, wy0 + pad)
    # beside it: the name and what it is
    tx = wx1 + 44
    tw = cx1 - 44 - tx
    y = wy0 - 6
    d.text((tx, y), "exe-planet", font=bold, fill=BLACK, stroke_width=HEAVY, stroke_fill=BLACK)
    y += 76
    tagline = ("A static site builder on a Mac OS 9 desk in your browser. "
               "Write in Markdown; every save rebuilds the site with the Planet app's own templates.")
    for line in wrap(d, tagline, body, tw):
        d.text((tx, y), line, font=body, fill=BLACK)
        y += 36
    # under both: the three things it does, full width, centred in the
    # room between the badge and the status strip; a line that would run
    # off the frame is a mistake, not a wrap
    points = ["Sites are folders in your Workspace, a folder a post, its pictures beside it.",
              "One switch puts a site on the web under your name.",
              "Another sends every changed build to IPFS under a key the daemon keeps."]
    room_top, room_bottom = max(y, wy1), cy1 - sb_h
    y = room_top + (room_bottom - room_top - 40 * len(points)) // 2
    for line in points:
        if d.textlength(line, font=body) > cx1 - 44 - (wx0 + 24):
            raise SystemExit("too long for the card: " + line)
        d.rectangle([wx0 + 2, y + 13, wx0 + 9, y + 20], fill=HL)
        d.text((wx0 + 24, y), line, font=body, fill=BLACK)
        y += 40
    return im, badge_at, scale


def main():
    badge = Image.open(BADGE)
    frames = [f.convert("RGB").copy() for f in ImageSequence.Iterator(badge)]
    durations = []
    badge.seek(0)
    for f in ImageSequence.Iterator(badge):
        durations.append(f.info.get("duration", 300))
    base, (bx, by), scale = base_frame()
    # one palette for every frame: the badge's colours, the chrome, and
    # the grays the text's edges need, so nothing dithers or shifts
    pal = []
    for c in sorted({p for p in frames[0].getdata()}):
        pal.append(c)
    for c in [BLACK, G200, G300, G400, G600, G700, WHITE, HL, LAV, QUIET, (0x33,) * 3, (0xF1,) * 3]:
        if c not in pal:
            pal.append(c)
    for v in range(0x26, 0xFF, 4):
        c = (v, v, v)
        if c not in pal:
            pal.append(c)
    for v in range(0x33, 0xFF, 6):  # blue-to-white ramp for the highlight bullets' edges
        c = (int(0x33 + (0xFF - 0x33) * (v - 0x33) / (0xFF - 0x33)),) * 0 or (v, v, min(0xFF, 0x99 + (v - 0x33)))
        if c not in pal:
            pal.append(c)
    pal = pal[:256]
    palimg = Image.new("P", (1, 1))
    palimg.putpalette(sum([list(c) for c in pal], []) + [0] * (768 - 3 * len(pal)))
    out = []
    for fr in frames:
        im = base.copy()
        im.paste(fr.resize((88 * scale, 31 * scale), Image.NEAREST), (bx, by))
        out.append(im.quantize(palette=palimg, dither=Image.Dither.NONE))
    out[0].save(OUT, save_all=True, append_images=out[1:], duration=durations, loop=0, disposal=1, optimize=True)
    chk = Image.open(OUT)
    print(OUT, chk.size, chk.n_frames, "frames", os.path.getsize(OUT), "bytes", len({p for p in chk.convert("RGB").getdata()}), "colours in frame 1")
    # a still of the first frame for a look
    still = os.path.splitext(OUT)[0] + "-still.png"
    out[0].convert("RGB").save(still)
    print(still)


if __name__ == "__main__":
    main()
