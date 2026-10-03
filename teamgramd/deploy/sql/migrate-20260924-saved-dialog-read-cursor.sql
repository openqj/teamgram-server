ALTER TABLE `saved_dialogs`
  ADD `read_max_id` INT NOT NULL DEFAULT '0' AFTER `top_message`;
