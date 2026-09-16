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
crop('04_fig011.png', (1100,1450,2000,2080), 2, '_bl')
crop('04_fig011.png', (500,1500,1300,1900), 3, '_stray2')
