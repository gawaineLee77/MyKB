"""Deterministic graph protocol fixture; real native extraction/Neo4j, no live LLM."""
import json
import re
import sys
import threading
import time
from http.server import ThreadingHTTPServer
sys.path.insert(0, '/fixture')
from mock_openai import Handler

counts = {}
lock = threading.Lock()


class GraphModels(Handler):
    def log_message(self, *_): pass

    def do_GET(self):
        if self.path == '/counts':
            with lock: self._json(200, dict(counts))
        else: super().do_GET()

    def do_POST(self):
        with lock: counts[self.path] = counts.get(self.path, 0) + 1
        super().do_POST()

    def _chat(self, payload):
        messages = payload.get('messages', [])
        serialized = json.dumps(messages, ensure_ascii=False)
        if not payload.get('stream') and ('# Question' in serialized or 'entity_attributes' in serialized):
            user = next((m.get('content', '') for m in reversed(messages) if m.get('role') == 'user'), '')
            # Every output is derived solely from the synthetic fixture vocabulary.
            names = [name for name in ['Astra', 'Boreal', 'Helios', 'Vault'] if name in user]
            if not names: names = ['Astra']
            output = [{'entity': name, 'entity_attributes': ['synthetic component']} for name in names]
            if len(names) > 1:
                output.append({'entity1': names[0], 'entity2': names[1], 'relation': 'DEPENDS_ON'})
            with lock: counts['graph_extraction'] = counts.get('graph_extraction', 0) + 1
            self._json(200, {'id': 'graph-fixture', 'object': 'chat.completion', 'created': int(time.time()),
                'model': payload.get('model'), 'choices': [{'index': 0, 'message': {'role': 'assistant', 'content': json.dumps(output)}, 'finish_reason': 'stop'}],
                'usage': {'prompt_tokens': 10, 'completion_tokens': 10, 'total_tokens': 20}})
            return
        # A fixed answer alone is not proof of retrieval: the probe separately
        # requires graph-only source references and actual Neo4j nodes/edges.
        super()._chat(payload)


ThreadingHTTPServer(('0.0.0.0', 19090), GraphModels).serve_forever()
