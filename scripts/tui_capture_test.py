import unittest

from scripts import tui_capture


class DecodeKeySpecTests(unittest.TestCase):
    def test_literal_unicode_is_preserved(self):
        self.assertEqual(tui_capture.decode_key_spec("café · 中文 · 👩🏽‍💻"), "café · 中文 · 👩🏽‍💻".encode())

    def test_documented_ascii_escapes_are_decoded(self):
        self.assertEqual(tui_capture.decode_key_spec(r"a\n\x1bb"), b"a\n\x1bb")


class ProtocolSafetyTests(unittest.TestCase):
    def test_balanced_synchronized_output_is_allowed(self):
        tui_capture.assert_protocol_safety(b"before\x1b[?2026hframe\x1b[?2026lafter")

    def test_reversed_synchronized_output_is_rejected(self):
        with self.assertRaisesRegex(RuntimeError, "end without begin"):
            tui_capture.assert_protocol_safety(b"\x1b[?2026lbad\x1b[?2026h")

    def test_unclosed_synchronized_output_is_rejected(self):
        with self.assertRaisesRegex(RuntimeError, "was not restored"):
            tui_capture.assert_protocol_safety(b"\x1b[?2026hbad")

    def test_reversed_bracketed_paste_is_rejected(self):
        with self.assertRaisesRegex(RuntimeError, "bracketed-paste end without begin"):
            tui_capture.assert_protocol_safety(b"\x1b[?2004lbad\x1b[?2004h")

    def test_unclosed_bracketed_paste_is_rejected(self):
        with self.assertRaisesRegex(RuntimeError, "bracketed-paste mode was not restored"):
            tui_capture.assert_protocol_safety(b"\x1b[?2004hbad")

    def test_mouse_tracking_is_rejected(self):
        with self.assertRaisesRegex(RuntimeError, "mouse tracking"):
            tui_capture.assert_protocol_safety(b"\x1b[?1006h")

    def test_grouped_mouse_tracking_is_rejected(self):
        with self.assertRaisesRegex(RuntimeError, "mouse tracking"):
            tui_capture.assert_protocol_safety(b"\x1b[?1000;1006h")

    def test_grouped_alternate_screen_is_rejected(self):
        with self.assertRaisesRegex(RuntimeError, "alternate screen"):
            tui_capture.assert_protocol_safety(b"\x1b[?1049;2004h")

    def test_grouped_balanced_modes_are_allowed(self):
        tui_capture.assert_protocol_safety(
            b"\x1b[?2004;2026hframe\x1b[?2004;2026l"
        )

    def test_grouped_reversed_mode_is_rejected(self):
        with self.assertRaisesRegex(RuntimeError, "end without begin"):
            tui_capture.assert_protocol_safety(b"\x1b[?2004;2026lbad")


class SemanticRenderTests(unittest.TestCase):
    def test_resize_requires_text_after_resize_boundary(self):
        with self.assertRaisesRegex(RuntimeError, "post-resize stream omitted"):
            tui_capture.assert_semantic_text(
                "café".encode(), ["café"], b"repaint without semantic text"
            )

    def test_resize_accepts_text_in_both_segments(self):
        tui_capture.assert_semantic_text(
            "café".encode(), ["café"], "repaint café".encode()
        )


@unittest.skipIf(tui_capture.pyte is None, "pyte is not installed")
class SnapshotStyleTests(unittest.TestCase):
    def test_non_default_cell_styles_are_serialized(self):
        screen = tui_capture.pyte.Screen(10, 2)
        stream = tui_capture.pyte.Stream(screen)
        stream.feed("\x1b[38;2;1;2;3;1mHi\x1b[0m")

        snapshot = tui_capture.render_snapshot(screen)

        self.assertIn("Hi", snapshot)
        self.assertIn("-- cell styles --", snapshot)
        self.assertIn("fg=010203", snapshot)
        self.assertIn("bold", snapshot)


class ChildEnvTests(unittest.TestCase):
    def test_ci_is_removed_so_termenv_sees_a_terminal(self):
        # termenv reports "not a TTY" whenever CI is set, whatever the file
        # descriptor actually is, which strips every style span from the
        # capture and makes every golden look changed.
        env = tui_capture.child_env({"CI": "true", "PATH": "/usr/bin"})
        self.assertNotIn("CI", env)
        self.assertEqual(env["PATH"], "/usr/bin")

    def test_no_color_is_removed_even_when_zero(self):
        env = tui_capture.child_env({"NO_COLOR": "0"})
        self.assertNotIn("NO_COLOR", env)

    def test_colour_capability_is_pinned(self):
        env = tui_capture.child_env({"TERM": "dumb", "COLORTERM": ""})
        self.assertEqual(env["TERM"], "xterm-256color")
        self.assertEqual(env["COLORTERM"], "truecolor")
        self.assertEqual(env["CLICOLOR"], "1")


class AssertStyledTests(unittest.TestCase):
    def test_styled_capture_is_accepted(self):
        snapshot = "hi" + chr(10) + chr(10) + "-- cell styles --" + chr(10) + "1:1-2 fg=010203" + chr(10)
        tui_capture.assert_styled(snapshot, "packetcode", "plan")

    def test_styleless_capture_is_rejected(self):
        # Colour switched off yields text and no spans. Without this guard
        # `update` would promote that into the reviewed goldens.
        with self.assertRaises(RuntimeError) as caught:
            tui_capture.assert_styled("hi" + chr(10), "packetcode", "plan")
        self.assertIn("packetcode/plan", str(caught.exception))
        self.assertIn("colour disabled", str(caught.exception))

if __name__ == "__main__":
    unittest.main()
