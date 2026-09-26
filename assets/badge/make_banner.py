#!/usr/bin/env python3
"""exe-planet 88x31 web button.

A miniature Mac OS 9 Platinum window titled "exe-planet", its content a
starfield with the Planet app's ringed globe in the middle. Two stars
twinkle in turn; nothing else moves. Every pixel is placed by this script:
no fonts, no scaling, no dithering. Writes exe-planet-88x31.gif and
preview-8x.png next to itself, then re-opens the GIF and checks it.
"""
import os
from PIL import Image

HERE = os.path.dirname(os.path.abspath(__file__))
W, H = 88, 31

# ---- palette: Platinum greys + highlight, and the Planet icon's colours ----
PALETTE = [
    ('K',  (0x26, 0x26, 0x26)),  # Platinum black: borders, outlines, title text
    ('W',  (0xff, 0xff, 0xff)),  # bevel highlight, stripe rows, stars
    ('g3', (0xcc, 0xcc, 0xcc)),  # window body / title field
    ('g5', (0x99, 0x99, 0x99)),  # window bottom-right bevel
    ('g7', (0x77, 0x77, 0x77)),  # stripe rows, close-box shade
    ('HL', (0x33, 0x33, 0x99)),  # highlight blue: the "space" the window shows
    ('LV', (0xcc, 0xcc, 0xff)),  # lavender: dim stars, sparkle arms
    ('B1', (0x33, 0x66, 0xcc)),  # globe
    ('B2', (0x55, 0x88, 0xee)),  # globe's lit cap
    ('L1', (0x55, 0xaa, 0x55)),  # land
    ('L2', (0x88, 0xdd, 0x88)),  # land, lit edge
    ('R1', (0xe8, 0xc8, 0x80)),  # ring
    ('R2', (0xb0, 0x8a, 0x40)),  # ring shade
]
INDEX = {name: i for i, (name, _) in enumerate(PALETTE)}
RGB = dict(PALETTE)

# ---- lettering: a bold pixel face after Chicago/Charcoal -------------------
# 2px vertical stems, 1px horizontals, 5px x-height, 2px ascender, 1px
# descender (the title bar has no room for a second descender row).
# Rows: 0-1 ascender, 2-6 x-height, 7 descender.
def glyph(s):
    rows = s.strip('\n').split('\n')
    width = max(len(r) for r in rows)
    return [r.ljust(width, '.') for r in rows]

FONT = {
    'e': glyph("""
.....
.....
.###.
##.##
#####
##...
.####
....."""),
    'x': glyph("""
.....
.....
##.##
.###.
..#..
.###.
##.##
....."""),
    'p': glyph("""
.....
.....
####.
##.##
##.##
##.##
####.
##..."""),
    'l': glyph("""
##
##
##
##
##
##
##
.."""),
    'a': glyph("""
.....
.....
.###.
...##
.####
##.##
.####
....."""),
    'n': glyph("""
.....
.....
####.
##.##
##.##
##.##
##.##
....."""),
    't': glyph("""
....
.##.
####
.##.
.##.
.##.
..##
...."""),
    '-': glyph("""
....
....
....
....
####
....
....
...."""),
}
TRACKING = 1

# ---- the planet: the Planet app icon redrawn at 26x16 ----------------------
# o outline, b globe, c lit cap, w highlight, g land, h lit land,
# r ring, s ring shade, . transparent. The ring passes behind the globe
# above its middle and in front of it below.
PLANET = """
..........oooooo..........
........oocccccboo........
.......ocwwwccbbbbo.......
.......occccbhhgbbo.......
....ooocwccbhggggbbooo....
..oorocccbbbggggbbbboroo..
.orr.ocwcbbbbbbbbbbbo.rro.
or...ocbbbbbbbbbhgbbo...ro
or...obbhgbbbbbggggbo...ro
.orr.obbgbbbbbbbgggbo.rro.
..oorrrrrrrrrrrrrrssssoo..
....oosssrrrrrrrrrrroo....
......oooooooooooooo......
.......obbbbbbbbbbo.......
........oobbbbbboo........
..........oooooo..........
"""
PLANET_KEY = {'o': 'K', 'b': 'B1', 'c': 'B2', 'w': 'W', 'g': 'L1', 'h': 'L2',
              'r': 'R1', 's': 'R2'}
PLANET_AT = (31, 13)          # 26x16, centred in the 86x18 content area

# ---- stars ---------------------------------------------------------------
STARS_BRIGHT = [(4, 15), (24, 26), (63, 25), (76, 13), (83, 18)]
STARS_DIM = [(9, 25), (20, 14), (27, 20), (2, 21), (60, 15), (79, 27), (66, 20)]
TWINKLE = {'right': (71, 21), 'left': (13, 19)}
# L dim dot, W bright dot, X sparkle (white core, lavender arms)
SEQUENCE = [  # (right, left) per frame
    ('X', 'L'), ('W', 'L'), ('L', 'L'), ('L', 'W'),
    ('L', 'X'), ('L', 'W'), ('L', 'L'), ('W', 'L'),
]
FRAME_MS = 300


class Canvas:
    def __init__(self, fill):
        self.px = [[fill] * W for _ in range(H)]

    def set(self, x, y, c):
        if 0 <= x < W and 0 <= y < H:
            self.px[y][x] = c

    def rect(self, x, y, w, h, c):
        for yy in range(y, y + h):
            for xx in range(x, x + w):
                self.set(xx, yy, c)

    def hline(self, x0, x1, y, c):
        for x in range(x0, x1 + 1):
            self.set(x, y, c)

    def vline(self, x, y0, y1, c):
        for y in range(y0, y1 + 1):
            self.set(x, y, c)

    def frame(self, x, y, w, h, c):
        self.hline(x, x + w - 1, y, c)
        self.hline(x, x + w - 1, y + h - 1, c)
        self.vline(x, y, y + h - 1, c)
        self.vline(x + w - 1, y, y + h - 1, c)

    def image(self):
        im = Image.new('P', (W, H))
        flat = []
        for _, rgb in PALETTE:
            flat.extend(rgb)
        # a 16-entry palette: a 4-bit GIF, no 256-entry table per frame
        im.putpalette(flat + [0] * (16 * 3 - len(flat)))
        im.putdata([INDEX[c] for row in self.px for c in row])
        return im


def text_width(s):
    return sum(len(FONT[ch][0]) for ch in s) + TRACKING * (len(s) - 1)


def draw_text(cv, s, x, y, c):
    for ch in s:
        g = FONT[ch]
        for j, row in enumerate(g):
            for i, bit in enumerate(row):
                if bit == '#':
                    cv.set(x + i, y + j, c)
        x += len(g[0]) + TRACKING


def stripes(cv, x0, x1, y0, y1):
    """Title-bar grooves: 1px rows of white over #777, with the lighter end
    columns the desktop's .stripe::before / ::after draw."""
    for y in range(y0, y1 + 1):
        light = (y - y0) % 2 == 0
        cv.hline(x0, x1, y, 'W' if light else 'g7')
        cv.set(x0, y, 'W' if light else 'g3')
        cv.set(x1, y, 'g3' if light else 'g7')


def close_box(cv, x, y):
    """The 13px Platinum close box at 7px: a sunken outer ring (#777 top-left,
    white bottom-right), the black ring, and a raised 3x3 centre."""
    for i in range(7):
        cv.set(x + i, y, 'g7')
        cv.set(x, y + i, 'g7')
    for i in range(1, 7):
        cv.set(x + 6, y + i, 'W')
        cv.set(x + i, y + 6, 'W')
    cv.frame(x + 1, y + 1, 5, 5, 'K')
    cv.rect(x + 2, y + 2, 3, 3, 'g3')
    cv.hline(x + 2, x + 4, y + 2, 'W')
    cv.vline(x + 2, y + 2, y + 4, 'W')
    for p in ((x + 4, y + 3), (x + 4, y + 4), (x + 3, y + 4)):
        cv.set(*p, 'g7')


def star(cv, x, y, kind):
    if kind == 'L':
        cv.set(x, y, 'LV')
    elif kind == 'W':
        cv.set(x, y, 'W')
    elif kind == 'X':
        cv.set(x, y, 'W')
        for dx, dy in ((1, 0), (-1, 0), (0, 1), (0, -1)):
            cv.set(x + dx, y + dy, 'LV')


def draw_window(cv):
    SEAM = 11                                  # black line under the title bar
    cv.rect(0, 0, W, H, 'g3')
    cv.frame(0, 0, W, H, 'K')                  # 1px black window edge
    cv.hline(1, W - 3, 1, 'W')                 # bevel: white top-left...
    cv.vline(1, 1, SEAM - 1, 'W')
    cv.vline(W - 2, 1, SEAM - 1, 'g5')         # ...#999 bottom-right
    close_box(cv, 4, 3)
    # the title centres in the room right of the close box, as the flex row does
    title = 'exe-planet'
    field = text_width(title) + 4              # #ccc field, 2px each side
    left, right = 13, W - 5
    fx = left + ((right - left + 1) - field) // 2
    stripes(cv, left, fx - 1, 3, 8)
    stripes(cv, fx + field, right, 3, 8)
    draw_text(cv, title, fx + 2, 2, 'K')
    cv.hline(1, W - 2, SEAM, 'K')
    cv.rect(1, SEAM + 1, W - 2, H - 2 - SEAM, 'HL')   # content: space


def draw_planet(cv, ox, oy):
    rows = PLANET.strip('\n').split('\n')
    for y, row in enumerate(rows):
        for x, ch in enumerate(row):
            if ch in PLANET_KEY:
                cv.set(ox + x, oy + y, PLANET_KEY[ch])


def render(right, left):
    cv = Canvas('g3')
    draw_window(cv)
    for p in STARS_BRIGHT:
        star(cv, *p, 'W')
    for p in STARS_DIM:
        star(cv, *p, 'L')
    star(cv, *TWINKLE['right'], right)
    star(cv, *TWINKLE['left'], left)
    draw_planet(cv, *PLANET_AT)
    return cv.image()


def preview(frames, scale=8, gap=8):
    w = len(frames) * W * scale + (len(frames) - 1) * gap
    out = Image.new('RGB', (w, H * scale), (255, 255, 255))
    for i, f in enumerate(frames):
        big = f.convert('RGB').resize((W * scale, H * scale), Image.NEAREST)
        out.paste(big, (i * (W * scale + gap), 0))
    return out


def main():
    frames = [render(r, l) for r, l in SEQUENCE]
    gif = os.path.join(HERE, 'exe-planet-88x31.gif')
    frames[0].save(gif, save_all=True, append_images=frames[1:], loop=0,
                   duration=FRAME_MS, disposal=1, optimize=False)
    preview(frames).save(os.path.join(HERE, 'preview-8x.png'))

    # ---- verify: re-open and compare every decoded frame pixel for pixel ----
    allowed = set(RGB.values())
    im = Image.open(gif)
    assert im.size == (W, H), im.size
    n = im.n_frames
    assert n == len(frames), n
    colours, durations = set(), []
    for i in range(n):
        im.seek(i)
        got = im.convert('RGB')
        want = frames[i].convert('RGB')
        assert list(got.getdata()) == list(want.getdata()), f'frame {i} differs'
        used = set(got.getdata())
        assert used <= allowed, f'frame {i} has off-palette pixels: {used - allowed}'
        colours |= used
        durations.append(im.info.get('duration'))
    print(f'{os.path.basename(gif)}: size={im.size} frames={n} '
          f'loop={im.info.get("loop")} durations={durations} '
          f'bytes={os.path.getsize(gif)} colours={len(colours)} '
          f'(palette {len(PALETTE)}) exact-match=yes')
    pv = Image.open(os.path.join(HERE, 'preview-8x.png'))
    print(f'preview-8x.png: size={pv.size}')


if __name__ == '__main__':
    main()
