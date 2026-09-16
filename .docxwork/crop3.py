from PIL import Image
import os
base = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\out04\images"
out = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\crops"
def crop(name, box, scale, suffix):
    im = Image.open(os.path.join(base, name))
    c = im.crop(box)
    c = c.resize((int(c.width*scale), int(c.height*scale)), Image.LANCZOS)
    p = os.path.join(out, name.replace('.png','')+suffix+'.png')
    c.save(p); print(p, c.size)
crop('04_fig011.png', (500,1350,1300,1700), 2, '_band')
crop('04_fig016.png', (1000,700,1900,1042), 2, '_band')
