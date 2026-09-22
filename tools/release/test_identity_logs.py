import unittest
from collect_identity_logs import extract


class IdentityLogTests(unittest.TestCase):
    def test_compose_prefix_and_unwanted_fields(self):
        event = extract('gateway-1 | 2026/09/16 {"event":"identity_callback_checked","reason":"state_missing","cookie_present":true,"cookie":"SECRET","body":"PRIVATE"}')
        self.assertEqual(event, {'event': 'identity_callback_checked', 'reason': 'state_missing', 'cookie_present': True})

    def test_plain_upstream_errors_and_resource_logs_are_ignored(self):
        for line in ['SQL access_token=SECRET', '{"event":"http_request","path":"/api/v1/knowledge/PRIVATE"}', 'not JSON {', 'null', '{"event":null}']:
            with self.subTest(line=line):
                self.assertIsNone(extract(line))

    def test_auth_request_path_is_preserved(self):
        self.assertEqual(extract('{"event":"http_request","path":"/api/v1/auth/me","status":502}')['path'], '/api/v1/auth/me')


if __name__ == '__main__':
    unittest.main()
