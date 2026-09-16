p = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\arch.puml"
raw = open(p,'rb').read()
print("first 24 bytes:", raw[:24])
print("BOM:", raw[:3])
txt = raw.decode('utf-8-sig')
lines = txt.splitlines()
for i, l in enumerate(lines[:12], 1):
    print(i, repr(l[:90]))
