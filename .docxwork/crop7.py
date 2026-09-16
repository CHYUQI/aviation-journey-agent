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
crop('04_fig005.png', (760,20,1260,350), 4, '_dash')
crop('04_fig006.png', (60,650,700,1000), 3, '_useloc')
crop('04_fig002.png', (1560,30,2050,340), 4, '_reg2')
