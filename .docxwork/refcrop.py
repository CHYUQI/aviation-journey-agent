from PIL import Image
import os
src = r"C:\Users\96495\AppData\Local\Temp\codex-clipboard-4cfa0980-99a1-4dd3-b6c7-77c30a38f5e7.png"
out = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\ref"
os.makedirs(out, exist_ok=True)
im = Image.open(src).convert("RGB")
print("size", im.size)
w,h = im.size
def crop(name, box, scale=6):
    c = im.crop(box)
    c = c.resize((c.width*scale, c.height*scale), Image.LANCZOS)
    p = os.path.join(out, name)
    c.save(p); print(p, c.size)
crop("top.png", (0,0,int(w*0.55),int(h*0.28)))
crop("mid.png", (0,int(h*0.25),int(w*0.62),int(h*0.60)))
crop("right.png", (int(w*0.58),int(h*0.25),w,int(h*0.62)))
crop("bottom.png", (0,int(h*0.55),int(w*0.55),h))
crop("bottomright.png", (int(w*0.45),int(h*0.55),w,h))
