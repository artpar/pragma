import unittest

from harbor_pragma_agent import _is_fatal_pragma_exit, _is_provider_tools_turn_limit


class ProviderToolsTurnLimitTest(unittest.TestCase):
    def test_accepts_exact_terminal_turn_limit(self) -> None:
        output = (
            "[model response: model stop=tool_use]\n"
            "  ⏺ Bash\n"
            "error: provider tools loop exceeded maximum of 100 turns\n"
        )
        self.assertTrue(_is_provider_tools_turn_limit(output))

    def test_rejects_other_nonzero_failures(self) -> None:
        failures = (
            "error: provider requested tool use but returned no tool calls\n",
            "error: provider tools loop exceeded maximum of many turns\n",
            "error: provider tools loop exceeded maximum of 100 turns\nextra output\n",
            "Pragma returned no output",
        )
        for output in failures:
            with self.subTest(output=output):
                self.assertFalse(_is_provider_tools_turn_limit(output))

    def test_turn_limit_nonzero_is_not_fatal(self) -> None:
        output = "error: provider tools loop exceeded maximum of 100 turns\n"
        self.assertFalse(_is_fatal_pragma_exit(1, output))

    def test_other_nonzero_is_fatal_and_zero_is_not(self) -> None:
        output = "error: provider requested tool use but returned no tool calls\n"
        self.assertTrue(_is_fatal_pragma_exit(1, output))
        self.assertFalse(_is_fatal_pragma_exit(0, output))


if __name__ == "__main__":
    unittest.main()
