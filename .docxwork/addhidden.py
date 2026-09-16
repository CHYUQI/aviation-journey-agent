p = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\arch2.puml"
txt = open(p, encoding="utf-8").read()
anchor = "' ===== 关系 ====="
hidden = """' ===== 布局约束：强制六层纵向堆叠 =====
L1 -[hidden]down- L2
L2 -[hidden]down- L3
L3 -[hidden]down- L4
L4 -[hidden]down- L5
L5 -[hidden]down- L6

"""
txt = txt.replace(anchor, hidden + anchor)
open(r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\arch3.puml", "w", encoding="utf-8", newline="\n").write(txt)
print("ok")
