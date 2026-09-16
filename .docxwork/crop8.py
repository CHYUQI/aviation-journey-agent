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
crop('04_fig006.png', (20,360,780,650), 3, '_loc2')
crop('04_fig006.png', (20,30,780,340), 3, '_store2')
