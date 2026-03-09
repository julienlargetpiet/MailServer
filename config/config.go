package config

import (
	"fmt"
	"os"
)

type Config struct {
	Hostname string
	Domain   string

	MailRoot string

	SMTP SMTPConfig
	IMAP IMAPConfig
}

type SMTPConfig struct {
	ListenAddr string
}

type IMAPConfig struct {
	ListenAddr string
}

func Default() *Config {
	return &Config{
		Hostname: "mailserver",
		Domain:   "local",

		MailRoot: "./data/mail",

		SMTP: SMTPConfig{
			ListenAddr: ":2525",
		},

		IMAP: IMAPConfig{
			ListenAddr: ":2143",
		},
	}
}

func FromEnv() *Config {

	cfg := Default()

	if v := os.Getenv("MAIL_ROOT"); v != "" {
		cfg.MailRoot = v
	}

	if v := os.Getenv("MAIL_DOMAIN"); v != "" {
		cfg.Domain = v
	}

	if v := os.Getenv("SMTP_ADDR"); v != "" {
		cfg.SMTP.ListenAddr = v
	}

	if v := os.Getenv("IMAP_ADDR"); v != "" {
		cfg.IMAP.ListenAddr = v
	}

	return cfg
}

func (c *Config) String() string {
	return fmt.Sprintf(
		"hostname=%s domain=%s mailroot=%s smtp=%s imap=%s",
		c.Hostname,
		c.Domain,
		c.MailRoot,
		c.SMTP.ListenAddr,
		c.IMAP.ListenAddr,
	)
}

