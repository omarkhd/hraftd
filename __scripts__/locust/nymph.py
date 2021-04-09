import os


def gateway() -> str:
    out = os.popen("ip route | awk '/default/ { print $3 }'")
    return out.read()
