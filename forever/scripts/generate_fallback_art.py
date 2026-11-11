"""Build-time fallback art for a Windows-only Forever launcher.
The locally packaged preview uses the sharper user-approved full-window artwork.
"""
from PIL import Image, ImageDraw, ImageFont
from pathlib import Path
from math import sin
W,H=1536,1024
im=Image.new('RGB',(W,H))
px=im.load()
for y in range(H):
    for x in range(W):
        t=sin(x/130)+sin(y/90)
        if y<480: px[x,y]=(int(4+13*y/480),int(17+17*y/480),int(45+20*y/480+4*t))
        else: px[x,y]=(7,17,31)
d=ImageDraw.Draw(im,'RGBA')
for poly,color in [
    ([(0,450),(240,170),(500,440),(720,170),(990,480),(1200,200),(1536,460)],(73,92,142,255)),
    ([(0,480),(270,280),(480,465),(760,255),(1010,480),(1230,280),(1536,490)],(29,52,91,255)),
    ([(0,510),(340,370),(680,490),(1080,340),(1536,510)],(11,29,52,255))
]: d.polygon(poly,fill=color)
for x,y,w,h in [(710,185,120,230),(670,245,40,160),(830,250,40,155),(745,135,50,260),(765,95,10,80)]:
    d.rectangle((x,y,x+w,y+h),fill=(21,39,69,255),outline=(213,161,81,255),width=3)
    d.polygon([(x-8,y),(x+w//2,y-45),(x+w+8,y)],fill=(59,68,96,255),outline=(231,185,104,255))
for x,color in [(30,(32,99,177,255)),(1380,(170,46,49,255))]:
    d.polygon([(x,40),(x+120,40),(x+120,290),(x+60,334),(x,290)],fill=color,outline=(231,186,102,255),width=7)
    d.ellipse((x+30,85,x+90,160),outline=(232,192,92,255),width=9)
d.rectangle((0,0,W,52),fill=(3,11,24,245))
d.line((0,53,W,53),fill=(232,192,95,255),width=3)
for xy in [(34,505,1034,756),(1055,507,1510,759),(34,765,1036,910)]:
    d.rounded_rectangle(xy,radius=15,fill=(5,17,34,250),outline=(203,154,76,255),width=4)
fontdir=Path('C:/Windows/Fonts')
def font(n):
    p=fontdir/'georgia.ttf'
    return ImageFont.truetype(str(p),n) if p.exists() else ImageFont.load_default()
for x1,y1,x2,y2,txt in [
    (54,421,343,493,'Home'),(358,421,642,493,'Website'),
    (653,421,926,493,'Downloads'),(936,421,1207,493,'Updates'),
    (1215,421,1493,493,'Options')
]:
    d.rounded_rectangle((x1,y1,x2,y2),radius=9,fill=(13,42,77,253),outline=(213,174,100,255),width=4)
    d.text((x1+25,y1+15),txt,fill=(244,226,181,255),font=font(30))
d.rounded_rectangle((1068,780,1490,892),radius=18,fill=(21,111,39,255),outline=(234,191,97,255),width=8)
d.text((1195,806),'PLAY',fill=(251,240,204,255),font=font(57))
d.text((74,10),'DF   DawnForge Forever Launcher',fill=(234,201,127,255),font=font(25))
d.text((488,62),'DAWNFORGE',fill=(255,213,126,255),font=font(82),stroke_fill=(18,9,2,255),stroke_width=3)
d.text((610,160),'F O R E V E R',fill=(229,232,236,255),font=font(36))
target=Path(__file__).resolve().parents[1]/'app'/'artwork.jpg'
im.save(target,quality=94,optimize=True)
print(target)
