from PIL import Image
import os
base = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\out04\images"
out = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\crops"
os.makedirs(out, exist_ok=True)
def crop(name, box, scale, suffix):
    im = Image.open(os.path.join(base, name))
    c = im.crop(box)
    c = c.resize((int(c.width*scale), int(c.height*scale)), Image.LANCZOS)
    p = os.path.join(out, name.replace('.png','')+suffix+'.png')
    c.save(p); print(p, c.size)
crop('04_fig002.png', (340,80,700,235), 5, '_jh')
crop('04_fig002.png', (330,80,710,240), 5, '_rt')
crop('04_fig002.png', (1180,60,1760,320), 4, '_reg')
crop('04_fig002.png', (1740,60,2500,340), 3, '_ext')
crop('04_fig002.png', (0,150,400,330), 5, '_vue')
crop('04_fig002.png', (1180,180,1760,330), 4, '_store')
crop('04_fig011.png', (450,1100,850,1300), 5, '_stray')
crop('04_fig016.png', (950,780,1900,950), 3, '_stray')
