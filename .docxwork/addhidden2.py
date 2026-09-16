p = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\arch6.puml"
t = open(p, encoding="utf-8").read()
anchor = "' ===== 协作关系 ====="
hidden = ("L1 -[hidden]down- L2\n"
          "L2 -[hidden]down- L3\n"
          "L3 -[hidden]down- L4\n"
          "L4 -[hidden]down- L5\n"
          "L5 -[hidden]down- L6\n\n")
open(r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\arch7.puml", "w", encoding="utf-8", newline="\n").write(t.replace(anchor, hidden + anchor))
print("ok")
