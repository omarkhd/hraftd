import uuid

import locust

import nymph


class HraftdUser(locust.HttpUser):
    host = "http://" + nymph.gateway() + ":11000"
    wait_time = locust.between(0, 1)
    key_length = 4

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)

    @locust.task
    def foo(self) -> None:
        id4 = str(uuid.uuid4())
        k, v = id4[:self.key_length], id4
        self.client.post("/key", json={k: v})

    @locust.task
    def bar(self) -> None:
        k = str(uuid.uuid4())[:self.key_length]
        self.client.get("/key/" + k, name="/key")
