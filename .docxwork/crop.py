from PIL import Image
import os
base = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\out04\images"
out = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\crops"
os.makedirs(out, exist_ok=True)
def crop(name, box, scale=2, suffix=''):
    im = Image.open(os.path.join(base, name))
    w,h = im.size
    print(name, im.size)
    c = im.crop(box)
    c = c.resize((int(c.width*scale), int(c.height*scale)), Image.LANCZOS)
    p = os.path.join(out, name.replace('.png','')+suffix+'.png')
    c.save(p)
    print(' ->', p, c.size)
# fig002 full is very wide 2048x?; crop the middle-left region containing JourneyHandler
crop('04_fig002.png', (330,85,700,215), 4, '_a')
crop('04_fig002.png', (700,80,1250,250), 3, '_b')
